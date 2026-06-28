package requestctx

import (
	"context"
	"maps"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/types/consts"
)

// Metadata contains immutable request-scoped fields shared by logging and lower-level infrastructure.
type Metadata struct {
	route     string
	username  string
	userID    string
	sessionID string
	tenantID  string
	traceID   string
	params    map[string]string
	query     url.Values
}

// Fields contains request metadata fields for non-gin callers and tests.
type Fields struct {
	Route     string
	Username  string
	UserID    string
	SessionID string
	TenantID  string
	TraceID   string
	Params    map[string]string
	Query     url.Values
}

// New creates Metadata from explicit fields.
func New(fields Fields) Metadata {
	return Metadata{
		route:     fields.Route,
		username:  fields.Username,
		userID:    fields.UserID,
		sessionID: fields.SessionID,
		tenantID:  fields.TenantID,
		traceID:   fields.TraceID,
		params:    cloneStringMap(fields.Params),
		query:     cloneURLValues(fields.Query),
	}
}

// FromGin extracts Metadata from gin.Context.
func FromGin(c *gin.Context) Metadata {
	if c == nil {
		return Metadata{}
	}

	params := make(map[string]string)
	for _, key := range c.GetStringSlice(consts.PARAMS) {
		params[key] = c.Param(key)
	}

	var query url.Values
	if c.Request != nil && c.Request.URL != nil {
		query = c.Request.URL.Query()
	}

	return New(Fields{
		Route:     c.GetString(consts.CTX_ROUTE),
		Username:  c.GetString(consts.CTX_USERNAME),
		UserID:    c.GetString(consts.CTX_USER_ID),
		SessionID: c.GetString(consts.CTX_SESSION_ID),
		TenantID:  c.GetString(consts.CTX_TENANT_ID),
		TraceID:   c.GetString(consts.TRACE_ID),
		Params:    params,
		Query:     query,
	})
}

func (m Metadata) Route() string     { return m.route }
func (m Metadata) Username() string  { return m.username }
func (m Metadata) UserID() string    { return m.userID }
func (m Metadata) SessionID() string { return m.sessionID }
func (m Metadata) TenantID() string  { return m.tenantID }
func (m Metadata) TraceID() string   { return m.traceID }

func (m Metadata) Param(key string) string {
	if m.params == nil {
		return ""
	}
	return m.params[key]
}

func (m Metadata) Params() map[string]string { return cloneStringMap(m.params) }
func (m Metadata) Query() url.Values         { return cloneURLValues(m.query) }

type metadataContextKey struct{}

// WithMetadata returns a context carrying immutable request metadata.
func WithMetadata(ctx context.Context, meta Metadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	return context.WithValue(ctx, metadataContextKey{}, New(Fields{
		Route:     meta.Route(),
		Username:  meta.Username(),
		UserID:    meta.UserID(),
		SessionID: meta.SessionID(),
		TenantID:  meta.TenantID(),
		TraceID:   meta.TraceID(),
		Params:    meta.Params(),
		Query:     meta.Query(),
	}))
}

// FromContext extracts request metadata from ctx.
func FromContext(ctx context.Context) Metadata {
	if ctx == nil {
		return Metadata{}
	}

	meta, ok := ctx.Value(metadataContextKey{}).(Metadata)
	if !ok {
		return Metadata{}
	}
	return meta
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}

	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)
	return dst
}

func cloneURLValues(src url.Values) url.Values {
	if src == nil {
		return nil
	}

	dst := make(url.Values, len(src))
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}
