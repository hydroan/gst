package types

import (
	"context"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/sse"
	"github.com/hydroan/gst/types/consts"
)

var _ context.Context = (*ServiceContext)(nil)

// ServiceContext is the per-request context the framework hands to every
// service method. It implements context.Context by delegating to the request
// context, exposes request metadata (route, params, user identity, trace),
// and carries the response helpers a service needs without touching Gin
// directly.
type ServiceContext struct {
	baseCtx        context.Context
	ginCtx         *gin.Context
	responseWriter http.ResponseWriter

	request   *http.Request
	clientIP  string
	userAgent string

	phase        consts.Phase
	requiresAuth bool // indicates whether the current API requires authentication
}

// NewServiceContext builds a ServiceContext from the Gin request, capturing
// request details, phase, and user metadata.
//
// A non-nil ctx overrides the base context, which is how span tracing is
// propagated; when ctx is nil, the request context is used when available.
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

	serviceCtx := &ServiceContext{
		baseCtx:        ctx,
		ginCtx:         c,
		responseWriter: c.Writer,
		phase:          phase,
		requiresAuth:   c.GetBool(consts.CTX_REQUIRES_AUTH),
	}
	if c.Request != nil {
		serviceCtx.request = c.Request
		serviceCtx.clientIP = c.ClientIP()
		serviceCtx.userAgent = c.Request.UserAgent()
	}
	return serviceCtx
}

func (sc *ServiceContext) baseContext() context.Context {
	if sc == nil || sc.baseCtx == nil {
		return context.Background()
	}
	return sc.baseCtx
}

func (sc *ServiceContext) Deadline() (time.Time, bool) { return sc.baseContext().Deadline() }
func (sc *ServiceContext) Done() <-chan struct{}       { return sc.baseContext().Done() }
func (sc *ServiceContext) Err() error                  { return sc.baseContext().Err() }
func (sc *ServiceContext) Value(key any) any           { return sc.baseContext().Value(key) }

func (sc *ServiceContext) Phase() consts.Phase {
	if sc == nil {
		return ""
	}
	return sc.phase
}

// RequiresAuth returns whether the current API requires authentication.
func (sc *ServiceContext) RequiresAuth() bool {
	if sc == nil {
		return false
	}
	return sc.requiresAuth
}

func (sc *ServiceContext) Query() url.Values       { return requestctx.FromContext(sc).Query() }
func (sc *ServiceContext) Param(key string) string { return requestctx.FromContext(sc).Param(key) }
func (sc *ServiceContext) Route() string           { return requestctx.FromContext(sc).Route() }
func (sc *ServiceContext) Path() string            { return requestctx.FromContext(sc).Path() }
func (sc *ServiceContext) Method() string          { return requestctx.FromContext(sc).Method() }
func (sc *ServiceContext) Username() string        { return requestctx.FromContext(sc).Username() }
func (sc *ServiceContext) UserID() string          { return requestctx.FromContext(sc).UserID() }
func (sc *ServiceContext) SessionID() string       { return requestctx.FromContext(sc).SessionID() }

func (sc *ServiceContext) TenantID() string { return requestctx.FromContext(sc).TenantID() }
func (sc *ServiceContext) TraceID() string  { return requestctx.FromContext(sc).TraceID() }

func (sc *ServiceContext) Host() string {
	if sc == nil || sc.request == nil {
		return ""
	}
	return sc.request.Host
}

func (sc *ServiceContext) ClientIP() string {
	if sc == nil {
		return ""
	}
	return sc.clientIP
}

func (sc *ServiceContext) UserAgent() string {
	if sc == nil {
		return ""
	}
	return sc.userAgent
}

func (sc *ServiceContext) IsHTTPS() bool {
	if sc == nil || sc.request == nil {
		return false
	}
	if sc.request.TLS != nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(sc.request.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(sc.request.Header.Get("X-Forwarded-Ssl")), "on") {
		return true
	}
	return strings.Contains(strings.ToLower(sc.request.Header.Get("Forwarded")), "proto=https")
}

func (sc *ServiceContext) Data(code int, contentType string, data []byte) {
	if sc == nil || sc.ginCtx == nil {
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
// select on conn.Context().Done() to notice the client disconnecting.
//
// The error is fn's own error, or the setup failure that prevented streaming
// (reported before anything was written, so it still surfaces as a regular
// error response).
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
		return errors.New("service context carries no HTTP response to stream on")
	}
	return sse.Serve(sc.ginCtx.Writer, sc.ginCtx.Request, fn, opts...)
}

func (sc *ServiceContext) SetCookie(cookie *http.Cookie) {
	if sc == nil || sc.responseWriter == nil || cookie == nil {
		return
	}
	http.SetCookie(sc.responseWriter, cookie)
}

func (sc *ServiceContext) Cookie(name string) (string, error) {
	if sc == nil || sc.ginCtx == nil {
		return "", errors.New("service context has no gin context")
	}
	return sc.ginCtx.Cookie(name)
}

func (sc *ServiceContext) PostForm(key string) string {
	if sc == nil || sc.ginCtx == nil {
		return ""
	}
	return sc.ginCtx.PostForm(key)
}

func (sc *ServiceContext) FormFile(name string) (*multipart.FileHeader, error) {
	if sc == nil || sc.ginCtx == nil {
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
// seeding, a scheduled job, framework code. Inside a request it cannot be
// empty, because authorization refuses anonymous requests before any handler
// runs.
func RequestUserID(ctx context.Context) string {
	return requestctx.FromContext(ctx).UserID()
}
