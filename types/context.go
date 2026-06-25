package types

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/sse"
	"github.com/hydroan/gst/types/consts"
)

var _ context.Context = (*ServiceContext)(nil)

type ServiceContext struct {
	method       string
	request      *http.Request
	url          *url.URL
	header       http.Header
	writerHeader http.Header
	clientIP     string
	userAgent    string

	context context.Context
	writer  http.ResponseWriter
	// Body    []byte

	ginCtx       *gin.Context
	phase        consts.Phase
	requiresAuth bool // indicates whether the current API requires authentication
}

// NewServiceContext creates ServiceContext from gin.Context.
// Including request details, headers and user information.
//
// You can pass the custom context.Context to propagate span tracing,
// otherwise use the c.Request.Context().
func NewServiceContext(c *gin.Context, ctxs ...context.Context) *ServiceContext {
	if c == nil {
		return &ServiceContext{context: context.Background()}
	}

	ctx := c.Request.Context()
	if len(ctxs) > 0 && ctxs[0] != nil {
		ctx = ctxs[0]
	}
	meta := NewRequestMetadata(c)
	ctx = ContextWithRequestMetadata(ctx, meta)

	return &ServiceContext{
		request: c.Request,

		method:       c.Request.Method,
		url:          c.Request.URL,
		header:       c.Request.Header,
		writerHeader: c.Writer.Header(),
		clientIP:     c.ClientIP(),
		userAgent:    c.Request.UserAgent(),

		ginCtx:  c,
		context: ctx,
		writer:  c.Writer,

		// Check if the current route requires authentication
		// Determined by checking if there's a flag set by authentication middleware in gin.Context
		requiresAuth: c.GetBool(consts.CTX_REQUIRES_AUTH),
	}
}

// Context returns sc as context.Context.
func (sc *ServiceContext) Context() context.Context {
	if sc == nil {
		return context.Background()
	}
	return sc
}

func (sc *ServiceContext) baseContext() context.Context {
	if sc == nil || sc.context == nil {
		return context.Background()
	}
	return sc.context
}

func (sc *ServiceContext) Deadline() (time.Time, bool) { return sc.baseContext().Deadline() }
func (sc *ServiceContext) Done() <-chan struct{}       { return sc.baseContext().Done() }
func (sc *ServiceContext) Err() error                  { return sc.baseContext().Err() }
func (sc *ServiceContext) Value(key any) any           { return sc.baseContext().Value(key) }

func (sc *ServiceContext) RequestMetadata() RequestMetadata {
	return RequestMetadataFromContext(sc)
}

func (sc *ServiceContext) Method() string {
	if sc == nil {
		return ""
	}
	return sc.method
}

func (sc *ServiceContext) Request() *http.Request {
	if sc == nil || sc.request == nil {
		return nil
	}
	return sc.request.Clone(sc.baseContext())
}

func (sc *ServiceContext) URL() *url.URL {
	if sc == nil || sc.url == nil {
		return nil
	}
	u := *sc.url
	return &u
}

func (sc *ServiceContext) Header() http.Header {
	if sc == nil || sc.header == nil {
		return nil
	}
	return sc.header.Clone()
}

func (sc *ServiceContext) WriterHeader() http.Header {
	if sc == nil || sc.writerHeader == nil {
		return nil
	}
	return sc.writerHeader.Clone()
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

func (sc *ServiceContext) ResponseWriter() http.ResponseWriter {
	if sc == nil {
		return nil
	}
	return sc.writer
}

func (sc *ServiceContext) Params() map[string]string { return sc.RequestMetadata().Params() }
func (sc *ServiceContext) Query() url.Values         { return sc.RequestMetadata().Query() }
func (sc *ServiceContext) Param(key string) string   { return sc.RequestMetadata().Param(key) }
func (sc *ServiceContext) Route() string             { return sc.RequestMetadata().Route() }
func (sc *ServiceContext) Username() string          { return sc.RequestMetadata().Username() }
func (sc *ServiceContext) UserID() string            { return sc.RequestMetadata().UserID() }
func (sc *ServiceContext) SessionID() string         { return sc.RequestMetadata().SessionID() }
func (sc *ServiceContext) TraceID() string           { return sc.RequestMetadata().TraceID() }

func (sc *ServiceContext) WithRequestMetadata(meta RequestMetadata) *ServiceContext {
	next := sc.clone()
	next.context = ContextWithRequestMetadata(next.baseContext(), meta)
	return next
}

func (sc *ServiceContext) Data(code int, contentType string, data []byte) {
	sc.ginCtx.Data(code, contentType, data)
}

func (sc *ServiceContext) HTML(code int, name string, obj any) {
	sc.ginCtx.HTML(code, name, obj)
}

func (sc *ServiceContext) Redirect(code int, location string) {
	sc.ginCtx.Redirect(code, location)
}

func (sc *ServiceContext) SetCookie(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
	sc.ginCtx.SetCookie(name, value, maxAge, path, domain, secure, httpOnly)
}

func (sc *ServiceContext) Cookie(name string) (string, error) {
	return sc.ginCtx.Cookie(name)
}

func (sc *ServiceContext) PostForm(key string) string {
	return sc.ginCtx.PostForm(key)
}

func (sc *ServiceContext) FormFile(name string) (*multipart.FileHeader, error) {
	return sc.ginCtx.FormFile(name)
}

func (sc *ServiceContext) SSEvent(name string, message any) {
	sc.ginCtx.SSEvent(name, message)
}

func (sc *ServiceContext) Stream(step func(io.Writer) bool) {
	sc.ginCtx.Stream(step)
}

func (sc *ServiceContext) GetPhase() consts.Phase {
	if sc == nil {
		return ""
	}
	return sc.phase
}

func (sc *ServiceContext) WithPhase(phase consts.Phase) *ServiceContext {
	next := sc.clone()
	next.phase = phase
	return next
}

// RequiresAuth returns whether the current API requires authentication
func (sc *ServiceContext) RequiresAuth() bool {
	if sc == nil {
		return false
	}
	return sc.requiresAuth
}

func (sc *ServiceContext) clone() *ServiceContext {
	if sc == nil {
		return &ServiceContext{context: context.Background()}
	}
	next := *sc
	if sc.header != nil {
		next.header = sc.header.Clone()
	}
	if sc.writerHeader != nil {
		next.writerHeader = sc.writerHeader.Clone()
	}
	if next.context == nil {
		next.context = context.Background()
	}
	return &next
}

// Encode writes an SSE event to the given writer.
// This is a convenience method that wraps sse.Encode for use within
// SSE stream callbacks.
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
//	ctx.SSE().WithInterval(1*time.Second).Stream(func(w io.Writer) bool {
//	    _ = ctx.Encode(w, types.Event{
//	        Event: "message",
//	        Data:  "Hello",
//	    })
//	    return true
//	})
//
// Parameters:
//   - w: Writer to write the event to (typically from SSE stream callback)
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
