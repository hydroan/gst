package types

import (
	"context"
	"maps"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/types/consts"
)

// RequestMetadata contains immutable request-scoped fields shared by logging
// and lower-level infrastructure.
type RequestMetadata struct {
	route     string
	username  string
	userID    string
	sessionID string
	traceID   string
	params    map[string]string
	query     url.Values
}

// RequestMetadataFields contains request metadata fields for non-gin callers
// and tests.
type RequestMetadataFields struct {
	Route     string
	Username  string
	UserID    string
	SessionID string
	TraceID   string
	Params    map[string]string
	Query     url.Values
}

type requestMetadataContextKey struct{}

// RequestMetadataFromGin extracts RequestMetadata from gin.Context.
func RequestMetadataFromGin(c *gin.Context) RequestMetadata {
	if c == nil {
		return RequestMetadata{}
	}

	params := make(map[string]string)
	for _, key := range c.GetStringSlice(consts.PARAMS) {
		params[key] = c.Param(key)
	}

	var query url.Values
	if c.Request != nil && c.Request.URL != nil {
		query = c.Request.URL.Query()
	}

	return NewRequestMetadata(RequestMetadataFields{
		Route:     c.GetString(consts.CTX_ROUTE),
		Username:  c.GetString(consts.CTX_USERNAME),
		UserID:    c.GetString(consts.CTX_USER_ID),
		SessionID: c.GetString(consts.CTX_SESSION_ID),
		TraceID:   c.GetString(consts.TRACE_ID),
		Params:    params,
		Query:     query,
	})
}

// NewRequestMetadata creates RequestMetadata from explicit fields.
func NewRequestMetadata(fields RequestMetadataFields) RequestMetadata {
	return RequestMetadata{
		route:     fields.Route,
		username:  fields.Username,
		userID:    fields.UserID,
		sessionID: fields.SessionID,
		traceID:   fields.TraceID,
		params:    cloneStringMap(fields.Params),
		query:     cloneURLValues(fields.Query),
	}
}

// ContextWithRequestMetadata returns a context carrying immutable request metadata.
func ContextWithRequestMetadata(ctx context.Context, meta RequestMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	return context.WithValue(ctx, requestMetadataContextKey{}, NewRequestMetadata(RequestMetadataFields{
		Route:     meta.Route(),
		Username:  meta.Username(),
		UserID:    meta.UserID(),
		SessionID: meta.SessionID(),
		TraceID:   meta.TraceID(),
		Params:    meta.Params(),
		Query:     meta.Query(),
	}))
}

// RequestMetadataFromContext extracts request metadata from ctx.
func RequestMetadataFromContext(ctx context.Context) RequestMetadata {
	if ctx == nil {
		return RequestMetadata{}
	}

	meta, ok := ctx.Value(requestMetadataContextKey{}).(RequestMetadata)
	if !ok {
		return RequestMetadata{}
	}
	return meta
}

func (m RequestMetadata) Route() string     { return m.route }
func (m RequestMetadata) Username() string  { return m.username }
func (m RequestMetadata) UserID() string    { return m.userID }
func (m RequestMetadata) SessionID() string { return m.sessionID }
func (m RequestMetadata) TraceID() string   { return m.traceID }

func (m RequestMetadata) Param(key string) string {
	if m.params == nil {
		return ""
	}
	return m.params[key]
}

func (m RequestMetadata) Params() map[string]string { return cloneStringMap(m.params) }
func (m RequestMetadata) Query() url.Values         { return cloneURLValues(m.query) }

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
