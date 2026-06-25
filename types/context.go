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

type ServiceContext struct {
	Method       string        // http method
	Request      *http.Request // http request
	URL          *url.URL      // request url
	Header       http.Header   // http request header
	WriterHeader http.Header   // http writer header
	ClientIP     string        // client ip
	UserAgent    string        // user agent

	context context.Context
	Writer  http.ResponseWriter
	// Body    []byte

	// route parameters,
	//
	// eg: PUT /api/gists/:id/star
	// Params: map[string]string{"id": "xxxxx-mygistid-xxxxx"}
	//
	// eg: DELETE /api/user/:userid/shelf/:shelfid/book
	// Params: map[string]string{"userid": "xxxxx-myuserid-xxxxx", "shelfid": "xxxxx-myshelfid-xxxxx"}
	Params map[string]string
	Query  url.Values

	SessionID string // session id
	Username  string // currrent login user.
	UserID    string // currrent login user id
	Route     string

	TraceID string
	PSpanID string
	SpanID  string
	Seq     int

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
		Request: c.Request,

		Method:       c.Request.Method,
		URL:          c.Request.URL,
		Header:       c.Request.Header,
		WriterHeader: c.Writer.Header(),
		ClientIP:     c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		Params:       meta.Params(),
		Query:        meta.Query(),

		Route:     meta.Route(),
		Username:  meta.Username(),
		UserID:    meta.UserID(),
		SessionID: meta.SessionID(),

		TraceID: meta.TraceID(),

		ginCtx:  c,
		context: ctx,
		Writer:  c.Writer,

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

func (sc *ServiceContext) SetPhase(phase consts.Phase) { sc.phase = phase }
func (sc *ServiceContext) GetPhase() consts.Phase      { return sc.phase }
func (sc *ServiceContext) WithPhase(phase consts.Phase) *ServiceContext {
	sc.phase = phase
	return sc
}

// RequiresAuth returns whether the current API requires authentication
func (sc *ServiceContext) RequiresAuth() bool { return sc.requiresAuth }

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

type ModelContext struct {
	context context.Context
}

func NewModelContext(ctx context.Context) *ModelContext {
	if ctx == nil {
		ctx = context.Background()
	}

	return &ModelContext{context: ctx}
}

func (mc *ModelContext) Context() context.Context {
	if mc == nil {
		return context.Background()
	}
	return mc
}

func (mc *ModelContext) baseContext() context.Context {
	if mc == nil || mc.context == nil {
		return context.Background()
	}
	return mc.context
}

func (mc *ModelContext) Deadline() (time.Time, bool) { return mc.baseContext().Deadline() }
func (mc *ModelContext) Done() <-chan struct{}       { return mc.baseContext().Done() }
func (mc *ModelContext) Err() error                  { return mc.baseContext().Err() }
func (mc *ModelContext) Value(key any) any           { return mc.baseContext().Value(key) }
