package types

import (
	"context"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/sse"
)

var _ context.Context = (*ServiceContext)(nil)

// ServiceContext is the per-request context the framework hands to every
// service method. It implements context.Context by delegating to the request
// context, exposes request metadata (route, params, user identity, trace,
// whether the action requires authentication), and carries the response
// helpers a service needs without touching Gin directly.
type ServiceContext struct {
	baseCtx        context.Context
	ginCtx         *gin.Context
	responseWriter http.ResponseWriter

	phase consts.Phase

	// httpOnlyMethodCalled records that a service called a method only an
	// HTTP request can serve (see HTTPOnlyMethods) while the context carried
	// none; HTTPOnlyMethodCalled reads it.
	httpOnlyMethodCalled bool
}

// NewServiceContext builds a ServiceContext from the Gin request, capturing
// request details, phase, and user metadata.
//
// A non-nil ctx overrides the base context, which is how span tracing is
// propagated; when ctx is nil, the request context is used when available.
//
// Without a Gin request (c == nil) the context answers request metadata from
// ctx alone: a transport other than HTTP attaches requestctx.Metadata to ctx
// before building the context, and a context built on a bare ctx reads empty
// metadata.
//
// NewServiceContext always returns a non-nil *ServiceContext, even when
// both c and ctx are nil. ServiceContext methods are also nil-receiver
// safe and return zero values on a nil receiver, so callers never need
// defensive nil checks around the returned context.
//
//nolint:revive // ServiceContext is constructed from the Gin request first.
func NewServiceContext(c *gin.Context, ctx context.Context, phase consts.Phase) *ServiceContext {
	if c == nil {
		if ctx == nil {
			ctx = context.Background()
		}
		return &ServiceContext{baseCtx: ctx, phase: phase}
	}

	if ctx == nil {
		ctx = context.Background()
		if c.Request != nil {
			ctx = c.Request.Context()
		}
	}
	ctx = requestctx.WithMetadata(ctx, requestctx.FromGin(c))

	return &ServiceContext{
		baseCtx:        ctx,
		ginCtx:         c,
		responseWriter: c.Writer,
		phase:          phase,
	}
}

// baseContext returns the context the ServiceContext delegates to, or the
// background context on a nil or empty receiver.
func (sc *ServiceContext) baseContext() context.Context {
	if sc == nil || sc.baseCtx == nil {
		return context.Background()
	}
	return sc.baseCtx
}

// Deadline, Done, Err and Value delegate to the request context, which is
// what lets a ServiceContext stand wherever a context.Context is expected.
func (sc *ServiceContext) Deadline() (time.Time, bool) { return sc.baseContext().Deadline() }
func (sc *ServiceContext) Done() <-chan struct{}       { return sc.baseContext().Done() }
func (sc *ServiceContext) Err() error                  { return sc.baseContext().Err() }
func (sc *ServiceContext) Value(key any) any           { return sc.baseContext().Value(key) }

// Phase returns the action phase the service is invoked for.
func (sc *ServiceContext) Phase() consts.Phase {
	if sc == nil {
		return ""
	}
	return sc.phase
}

// Query, Param, Route, Path, Method, Username, UserID, SessionID and
// RequiresAuth read the request metadata captured when the context was
// built. Query returns a copy, so mutating the result never changes what a
// later read sees; RequiresAuth answers whether the action requires
// authentication, as the transport marked the request or the call. Over
// gRPC, Route and Method name the action the way the registration described
// it, the route and HTTP method it is served at over HTTP, Path is the full
// method of the call, and Query is empty for every action but a List or a
// Get, whose query the request message carries: the input of any other
// action travels in its payload.
func (sc *ServiceContext) Query() url.Values       { return requestctx.FromContext(sc).Query() }
func (sc *ServiceContext) Param(key string) string { return requestctx.FromContext(sc).Param(key) }
func (sc *ServiceContext) Route() string           { return requestctx.FromContext(sc).Route() }
func (sc *ServiceContext) Path() string            { return requestctx.FromContext(sc).Path() }
func (sc *ServiceContext) Method() string          { return requestctx.FromContext(sc).Method() }
func (sc *ServiceContext) Username() string        { return requestctx.FromContext(sc).Username() }
func (sc *ServiceContext) UserID() string          { return requestctx.FromContext(sc).UserID() }
func (sc *ServiceContext) SessionID() string       { return requestctx.FromContext(sc).SessionID() }
func (sc *ServiceContext) RequiresAuth() bool      { return requestctx.FromContext(sc).RequiresAuth() }

// TenantID reads the tenant the request was authenticated into, and TraceID
// the trace id of the execution the context belongs to.
func (sc *ServiceContext) TenantID() string { return requestctx.FromContext(sc).TenantID() }
func (sc *ServiceContext) TraceID() string  { return execctx.FromContext(sc).TraceID }

// Host, ClientIP, UserAgent and IsHTTPS describe the connection the request
// arrived on, read from the same metadata: the host the request was addressed
// to, the client address Gin resolved (forwarding headers included), the
// User-Agent header, and whether the request arrived over TLS, either
// directly or as the forwarding headers of a proxy in front declare.
func (sc *ServiceContext) Host() string      { return requestctx.FromContext(sc).Host() }
func (sc *ServiceContext) ClientIP() string  { return requestctx.FromContext(sc).ClientIP() }
func (sc *ServiceContext) UserAgent() string { return requestctx.FromContext(sc).UserAgent() }
func (sc *ServiceContext) IsHTTPS() bool     { return requestctx.FromContext(sc).TLS() }

// HTTPOnlyMethods names the methods of ServiceContext that only an HTTP
// request can serve, sorted: Cookie, PostForm and FormFile read what only
// an HTTP request carries, Data and SSE write the response themselves, and
// SetCookie writes a response header. Over gRPC they have nothing to work
// on, so the services of a model declaring GRPC() must not call them; gg
// check holds them to it (its check named gRPC service context), and a
// call that reaches one at run time records it for HTTPOnlyMethodCalled,
// which the transport answers as a failure once the hook or the service
// method returns.
var HTTPOnlyMethods = []string{"Cookie", "Data", "FormFile", "PostForm", "SSE", "SetCookie"}

// Data writes data as the response body with the given status and content
// type. Without a response to write to it writes nothing and records the
// call for HTTPOnlyMethodCalled.
func (sc *ServiceContext) Data(code int, contentType string, data []byte) {
	if sc == nil || sc.ginCtx == nil {
		sc.recordHTTPOnlyMethodCall()
		return
	}
	sc.ginCtx.Data(code, contentType, data)
}

// SSE turns the response into a Server-Sent Events stream and runs fn with
// the live connection.
//
// The framework owns the connection lifecycle: it clears the server's
// per-request deadlines so the stream outlives the global WriteTimeout,
// writes and flushes the SSE response headers, sends keep-alive comment
// frames until fn returns, and invalidates the connection afterwards. fn
// blocks until the stream is over; a callback that waits for events must
// select on conn.Context().Done() to notice the client disconnecting or the
// server shutting down.
//
// The error is fn's own error, or the setup failure that prevented streaming
// (reported before anything was written, so it still surfaces as a regular
// error response). Without a response to stream on it records the attempt for
// HTTPOnlyMethodCalled and reports the failure.
//
// Example:
//
//	return nil, ctx.SSE(func(conn *sse.Conn) error {
//		for {
//			select {
//			case <-conn.Context().Done():
//				return nil
//			case event := <-events:
//				if err := conn.Send(event); err != nil {
//					return err
//				}
//			}
//		}
//	})
func (sc *ServiceContext) SSE(fn func(conn *sse.Conn) error, opts ...sse.Option) error {
	if sc == nil || sc.ginCtx == nil {
		sc.recordHTTPOnlyMethodCall()
		return errors.New("service context carries no HTTP response to stream on")
	}
	return sse.Serve(sc.ginCtx.Writer, sc.ginCtx.Request, fn, opts...)
}

// SetCookie adds cookie to the response headers. Without a response to write
// to it writes nothing and records the call for HTTPOnlyMethodCalled; a
// nil cookie is nothing to write on any transport and records nothing.
func (sc *ServiceContext) SetCookie(cookie *http.Cookie) {
	if sc == nil || cookie == nil {
		return
	}
	if sc.responseWriter == nil {
		sc.recordHTTPOnlyMethodCall()
		return
	}
	http.SetCookie(sc.responseWriter, cookie)
}

// recordHTTPOnlyMethodCall marks that a service called a method only an HTTP
// request can serve while the context carried none; a nil context has
// nothing to mark.
func (sc *ServiceContext) recordHTTPOnlyMethodCall() {
	if sc != nil {
		sc.httpOnlyMethodCalled = true
	}
}

// HTTPOnlyMethodCalled reports whether a service called a method of sc only
// an HTTP request can serve (see HTTPOnlyMethods) -- wrote a body, a stream
// or a cookie, or read a cookie, a form value or a file -- while sc carried
// no HTTP request or response. The call itself does nothing, answering the
// zero value where it reads; the transport behind such a context reads the
// flag once the hook or the service method returns and refuses the request,
// because what the service meant to send cannot be carried and what it
// meant to read was never there. It is a package function rather than a
// method so it stays out of the public alias of ServiceContext: only the
// framework's transports read it.
func HTTPOnlyMethodCalled(sc *ServiceContext) bool {
	return sc != nil && sc.httpOnlyMethodCalled
}

// Cookie returns the value of the named request cookie. Without a request
// to read it answers an error and records the call for
// HTTPOnlyMethodCalled.
func (sc *ServiceContext) Cookie(name string) (string, error) {
	if sc == nil || sc.ginCtx == nil {
		sc.recordHTTPOnlyMethodCall()
		return "", errors.New("service context has no gin context")
	}
	return sc.ginCtx.Cookie(name)
}

// PostForm returns the named value of the request's form body. Without a
// request to read it answers "" and records the call for
// HTTPOnlyMethodCalled.
func (sc *ServiceContext) PostForm(key string) string {
	if sc == nil || sc.ginCtx == nil {
		sc.recordHTTPOnlyMethodCall()
		return ""
	}
	return sc.ginCtx.PostForm(key)
}

// FormFile returns the named file of the request's multipart form. Without
// a request to read it answers an error and records the call for
// HTTPOnlyMethodCalled.
func (sc *ServiceContext) FormFile(name string) (*multipart.FileHeader, error) {
	if sc == nil || sc.ginCtx == nil {
		sc.recordHTTPOnlyMethodCall()
		return nil, errors.New("service context has no gin context")
	}
	return sc.ginCtx.FormFile(name)
}

// RequestUserID reports the authenticated subject of the request ctx descends
// from, or "" when no request is behind it.
//
// It exists for code that receives a plain context and still has to know who
// is acting — a model hook guarding an operation, like tenant.From for a model
// deriving its key. An empty answer means machinery rather than a person:
// seeding, a scheduled job, framework code; or a request nothing
// authenticated, a public action's or one of a project that mounts no
// session check (see iam.Register).
func RequestUserID(ctx context.Context) string {
	return requestctx.FromContext(ctx).UserID()
}
