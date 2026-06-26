package types

import (
	"context"
	"io"
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

type ServiceContext struct {
	baseCtx        context.Context
	ginCtx         *gin.Context
	responseWriter http.ResponseWriter

	request   *http.Request
	method    string
	clientIP  string
	userAgent string

	phase        consts.Phase
	requiresAuth bool // indicates whether the current API requires authentication
}

// NewServiceContext creates ServiceContext from gin.Context.
// Including request details, headers, phase, and user information.
//
// You can pass a custom context.Context to propagate span tracing.
// If ctx is nil, the request context is used when available.
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
		serviceCtx.method = c.Request.Method
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

func (sc *ServiceContext) Params() map[string]string { return requestctx.FromContext(sc).Params() }
func (sc *ServiceContext) Query() url.Values         { return requestctx.FromContext(sc).Query() }
func (sc *ServiceContext) Param(key string) string   { return requestctx.FromContext(sc).Param(key) }
func (sc *ServiceContext) Route() string             { return requestctx.FromContext(sc).Route() }
func (sc *ServiceContext) Username() string          { return requestctx.FromContext(sc).Username() }
func (sc *ServiceContext) UserID() string            { return requestctx.FromContext(sc).UserID() }
func (sc *ServiceContext) SessionID() string         { return requestctx.FromContext(sc).SessionID() }
func (sc *ServiceContext) TraceID() string           { return requestctx.FromContext(sc).TraceID() }
func (sc *ServiceContext) Method() string {
	if sc == nil {
		return ""
	}
	return sc.method
}

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

// Encode writes an SSE event to the given writer.
// This is a convenience method that wraps sse.Encode.
//
// The event is formatted according to the SSE specification:
//   - Fields are written in recommended order: id, event, retry, data
//   - Each field is written as "field: value\n"
//   - Multiple data fields are concatenated (for multi-line data)
//   - Events are separated by a blank line (\n\n)
//
// If Data is a complex type (map, struct, slice), it will be JSON-encoded.
// If Data is a primitive type, it will be converted to string.
// If Data is nil, no data field will be written.
//
// Example:
//
//	err := ctx.Encode(w, types.Event{
//		Event: "message",
//		Data:  "Hello",
//	})
//
// Parameters:
//   - w: Writer to write the event to
//   - event: SSE event to encode
//
// Returns:
//   - error: Any error that occurred during encoding
func (sc *ServiceContext) Encode(w io.Writer, event Event) error {
	if sc == nil {
		return nil
	}

	return sse.Encode(w, event)
}
