package requestctx

import (
	"context"
	"maps"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/types/consts"
)

// Metadata contains immutable request-scoped fields shared by logging and lower-level infrastructure.
//
// Route and path are two identities of the same request and both are kept:
// the route is the matched route pattern ("/api/users/:id") that groups every
// request of one endpoint together, the path is the concrete request path
// ("/api/users/42") that pins down a single request. Consumers aggregating
// requests must use the route; the path only locates one of them. The method
// is the request verb, which separates the actions one route pattern serves
// and neither of the other two distinguishes.
type Metadata struct {
	route     string
	path      string
	method    string
	username  string
	userID    string
	sessionID string
	tenantID  string
	traceID   string
	params    map[string]string
	query     url.Values
	rawQuery  string
}

// Fields contains request metadata fields for non-gin callers and tests.
//
// RawQuery is optional: when it is empty and Query is not, New re-encodes
// Query to fill it.
type Fields struct {
	Route     string
	Path      string
	Method    string
	Username  string
	UserID    string
	SessionID string
	TenantID  string
	TraceID   string
	Params    map[string]string
	Query     url.Values
	RawQuery  string
}

// New creates Metadata from explicit fields.
func New(fields Fields) Metadata {
	return Metadata{
		route:     fields.Route,
		path:      fields.Path,
		method:    fields.Method,
		username:  fields.Username,
		userID:    fields.UserID,
		sessionID: fields.SessionID,
		tenantID:  fields.TenantID,
		traceID:   fields.TraceID,
		params:    cloneStringMap(fields.Params),
		query:     cloneURLValues(fields.Query),
		rawQuery:  rawQueryOf(fields.RawQuery, fields.Query),
	}
}

// FromGin extracts Metadata from gin.Context.
//
// Route and path are read straight off the gin context instead of through
// context keys another middleware has to set first, so metadata is complete no
// matter where in the handler chain it is taken. The route is empty for
// requests gin did not match to a registered route (NoRoute, NoMethod); the
// path stays populated in those cases and identifies the request on its own.
//
// Metadata is constructed several times per request — the controller span,
// every service context, and every database handle each build one — so the
// expensive part, parsing the query string, is memoized on the gin context by
// GinQueryValues. Identity fields are deliberately read fresh on every call:
// they are cheap context lookups, and re-reading them keeps a construction
// that runs before the identity middleware from freezing empty identity into
// the constructions that follow it.
//
// The struct is built without New: New defensively clones the params and
// query maps of its caller, while both maps here are already owned — params
// is built below and the query values are the package's own memoized parse.
func FromGin(c *gin.Context) Metadata {
	if c == nil {
		return Metadata{}
	}

	// Routes declaring no parameters are the common case; leaving params nil
	// there keeps a map out of every one of their requests.
	var params map[string]string
	if keys := c.GetStringSlice(consts.PARAMS); len(keys) > 0 {
		params = make(map[string]string, len(keys))
		for _, key := range keys {
			params[key] = c.Param(key)
		}
	}

	var method, path, rawQuery string
	if c.Request != nil {
		method = c.Request.Method
		if c.Request.URL != nil {
			path = c.Request.URL.Path
			rawQuery = c.Request.URL.RawQuery
		}
	}

	return Metadata{
		route:     c.FullPath(),
		path:      path,
		method:    method,
		username:  c.GetString(consts.CTX_USERNAME),
		userID:    c.GetString(consts.CTX_USER_ID),
		sessionID: c.GetString(consts.CTX_SESSION_ID),
		tenantID:  c.GetString(consts.CTX_TENANT_ID),
		traceID:   c.GetString(consts.TRACE_ID),
		params:    params,
		query:     GinQueryValues(c),
		rawQuery:  rawQuery,
	}
}

// ginQueryKey keys the request's parsed query values on the gin context.
const ginQueryKey = "gst/requestctx/query"

// GinQueryValues returns the request's parsed query parameters, parsing them
// on the first call and reusing that result for the rest of the request.
//
// url.URL.Query re-parses the raw query string on every call. Only the parsed
// values are memoized, nothing else: they derive from the request URL alone,
// which cannot change for the lifetime of a request, so the memo is correct
// no matter where in the middleware chain the first call happens.
//
// The returned map is owned by this package and never written after it is
// stored. Metadata getters hand out clones, so callers of the public API
// cannot reach it; framework code reading it directly must treat it as
// read-only.
func GinQueryValues(c *gin.Context) url.Values {
	if c == nil {
		return nil
	}
	if cached, ok := c.Get(ginQueryKey); ok {
		if query, ok := cached.(url.Values); ok {
			return query
		}
	}
	if c.Request == nil || c.Request.URL == nil {
		return nil
	}
	query := c.Request.URL.Query()
	c.Set(ginQueryKey, query)
	return query
}

// ParseGinQuery parses the request's query string with the error url.URL.Query
// silently drops, and memoizes the parsed values on the gin context for
// GinQueryValues and everything built on it to reuse.
//
// It exists for the strict-query gate: the gate must see the parse error to
// reject malformed input, while every consumer after it should reuse the parse
// instead of paying for another one. The memo is written unconditionally — the
// gate runs before anything else parses the query, and overwriting with the
// authoritative parse keeps a memo an earlier lenient parse may have stored
// from surviving past the gate.
func ParseGinQuery(c *gin.Context) (url.Values, error) {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		// Nothing to parse and nothing to memoize; empty values keep the
		// no-request construction paths of tests working like GinQueryValues.
		return url.Values{}, nil
	}

	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		return nil, err
	}
	c.Set(ginQueryKey, query)

	return query, nil
}

func (m Metadata) Route() string     { return m.route }
func (m Metadata) Path() string      { return m.path }
func (m Metadata) Method() string    { return m.method }
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

// RawQuery returns the query string as the client sent it ("a=1&b=2").
//
// Request handling wants the query parsed into url.Values; logging wants the
// opposite. Logged as structured key-value pairs, every query key any client
// ever sends becomes a field in the log storage's mapping, and query keys are
// caller-controlled and unbounded: a misspelled parameter, a bracketed filter
// syntax and a scanner sending random parameters each add a permanent field,
// until the index reaches its field limit and silently rejects further
// entries. One raw string keeps the mapping at exactly one field, and keeps
// what was actually sent visible instead of a normalized reconstruction.
func (m Metadata) RawQuery() string { return m.rawQuery }

type metadataContextKey struct{}

// WithMetadata returns a context carrying immutable request metadata.
//
// The metadata is stored as given rather than rebuilt from its own getters.
// Metadata is immutable: its fields are unexported, nothing writes them after
// construction, and the two getters exposing maps hand out clones, so no
// caller can reach the stored maps to change them. Rebuilding here cloned
// those maps twice more on every request to produce a copy nothing could tell
// apart from the original.
func WithMetadata(ctx context.Context, meta Metadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	return context.WithValue(ctx, metadataContextKey{}, meta)
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

// QueryValues returns the request's parsed query parameters for read-only
// framework use, without the defensive copy Metadata.Query makes for
// arbitrary callers.
//
// The framework's own query parsers only read the values, and several of them
// run on every list request — one per query argument a database query is
// built from — so each paying for a deep clone would multiply the very cost
// the memoized parse removes. Callers must not modify the returned map; code
// outside the framework goes through Metadata.Query and gets a clone.
func QueryValues(ctx context.Context) url.Values {
	return FromContext(ctx).query
}

// rawQueryOf resolves the raw query string, re-encoding the parsed values for
// callers that only carry url.Values. Encoding normalizes key order and
// escaping, so gin-built metadata passes the original string instead.
func rawQueryOf(rawQuery string, query url.Values) string {
	if len(rawQuery) > 0 || len(query) == 0 {
		return rawQuery
	}
	return query.Encode()
}

// cloneStringMap copies src, returning nil when there is nothing to copy.
//
// The clone exists so callers cannot reach the metadata's own map. An empty
// source has nothing to protect, while allocating a map to hand back costs one
// allocation per request on the routes that declare no parameters — the common
// case. Nil is not a new state for readers to handle: metadata built outside a
// request carries nil here already, so every reader tolerates it. Reads on a
// nil map are well defined, and these clones are handed out to be read.
func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)
	return dst
}

// cloneURLValues copies src, returning nil when there is nothing to copy, for
// the reasons given on cloneStringMap.
func cloneURLValues(src url.Values) url.Values {
	if len(src) == 0 {
		return nil
	}

	dst := make(url.Values, len(src))
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}
