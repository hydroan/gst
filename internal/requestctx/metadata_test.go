package requestctx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

func TestFromGinExtractsRequestFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// The route comes from gin's own route matching, so the request has to go
	// through a registered route rather than a bare test context.
	var meta Metadata
	router := gin.New()
	router.GET("/api/users/:id", func(ctx *gin.Context) {
		ctx.Set(consts.PARAMS, []string{"id"})
		ctx.Set(consts.CTX_USERNAME, "admin")
		ctx.Set(consts.CTX_USER_ID, "user-1")
		ctx.Set(consts.CTX_SESSION_ID, "session-1")
		ctx.Set(consts.CTX_TENANT_ID, "tenant-1")

		meta = FromGin(ctx)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/users/42?tag=blue&tag=green&range=a[gte]", nil))

	require.Equal(t, "/api/users/:id", meta.Route())
	require.Equal(t, "/api/users/42", meta.Path())
	require.Equal(t, http.MethodGet, meta.Method())
	require.Equal(t, "admin", meta.Username())
	require.Equal(t, "user-1", meta.UserID())
	require.Equal(t, "session-1", meta.SessionID())
	require.Equal(t, "tenant-1", meta.TenantID())
	require.Equal(t, "42", meta.Param("id"))
	require.Equal(t, []string{"blue", "green"}, meta.Query()["tag"])
	// The raw query keeps key order and escaping exactly as sent, which
	// re-encoding the parsed values would not.
	require.Equal(t, "tag=blue&tag=green&range=a[gte]", meta.RawQuery())
}

func TestMetadataRawQueryFallsBackToEncodedQuery(t *testing.T) {
	meta := New(Fields{
		Query: url.Values{
			"tag":  {"blue", "green"},
			"name": {"sample"},
		},
	})

	require.Equal(t, "name=sample&tag=blue&tag=green", meta.RawQuery())
	require.Empty(t, New(Fields{}).RawQuery())
}

func TestMetadataProtectsParamsAndQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/users/42?tag=blue", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "42"}}
	ctx.Set(consts.PARAMS, []string{"id"})

	meta := FromGin(ctx)

	params := meta.Params()
	params["id"] = "mutated"
	query := meta.Query()
	query["tag"][0] = "mutated"

	require.Equal(t, "42", meta.Param("id"))
	require.Equal(t, []string{"blue"}, meta.Query()["tag"])
}

// TestFromGinReadsIdentityFreshWhileQueryStaysMemoized pins the split the
// query memo is built on: identity fields are read from the gin context on
// every construction, so a construction that runs before the identity
// middleware never freezes empty identity into the constructions that follow
// it, while the parsed query is one shared parse for the whole request.
func TestFromGinReadsIdentityFreshWhileQueryStaysMemoized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/users?tag=blue", nil)

	before := FromGin(ctx)
	require.Empty(t, before.Username(), "identity middleware has not run yet")
	require.Equal(t, []string{"blue"}, before.Query()["tag"])

	ctx.Set(consts.CTX_USERNAME, "admin")
	after := FromGin(ctx)
	require.Equal(t, "admin", after.Username(),
		"identity must be read fresh, not frozen by the first construction")
	require.Equal(t, []string{"blue"}, after.Query()["tag"])
}

// TestQueryValuesSharesTheMemoizedParse pins what QueryValues is for: the
// framework's read-only accessor returns the stored values without a clone,
// so every parser of one request reads the very same parse.
func TestQueryValuesSharesTheMemoizedParse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginCtx.Request = httptest.NewRequest(http.MethodGet, "/api/users?tag=blue&tag=green", nil)

	ctx := WithMetadata(context.Background(), FromGin(ginCtx))
	values := QueryValues(ctx)
	require.Equal(t, url.Values{"tag": {"blue", "green"}}, values)
	require.Equal(t, reflect.ValueOf(values).Pointer(), reflect.ValueOf(QueryValues(ctx)).Pointer(),
		"QueryValues must hand back the stored map, not a clone")
	require.Equal(t, reflect.ValueOf(values).Pointer(), reflect.ValueOf(GinQuery(ginCtx)).Pointer(),
		"every construction of one request must share one parse")
}

// TestGinQueryStrictMemoizesTheAuthoritativeParse pins the strict-query gate's
// contract with the memo: a successful parse is stored for GinQuery to
// reuse, and it overwrites whatever an earlier lenient parse may have stored,
// so everything after the gate reads the gate's own parse.
func TestGinQueryStrictMemoizesTheAuthoritativeParse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginCtx.Request = httptest.NewRequest(http.MethodGet, "/api/records?tag=blue", nil)

	// An earlier construction memoizes a lenient parse first.
	lenient := GinQuery(ginCtx)
	require.Equal(t, url.Values{"tag": {"blue"}}, lenient)

	parsed, err := GinQueryStrict(ginCtx)
	require.NoError(t, err)
	require.Equal(t, url.Values{"tag": {"blue"}}, parsed)
	require.NotEqual(t, reflect.ValueOf(lenient).Pointer(), reflect.ValueOf(parsed).Pointer(),
		"the gate parses unconditionally and overwrites the memo")
	require.Equal(t, reflect.ValueOf(parsed).Pointer(), reflect.ValueOf(GinQuery(ginCtx)).Pointer(),
		"everything after the gate must reuse the gate's parse")
}

// TestGinQueryStrictReportsMalformedWithoutMemoizing pins the failure side: the
// error url.URL.Query drops is surfaced, and no partial parse is stored.
func TestGinQueryStrictReportsMalformedWithoutMemoizing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginCtx.Request = httptest.NewRequest(http.MethodGet, "/api/records", nil)
	ginCtx.Request.URL.RawQuery = "a=%zz"

	_, err := GinQueryStrict(ginCtx)
	require.Error(t, err)
	_, stored := ginCtx.Get(ginQueryKey)
	require.False(t, stored, "a failed parse must not memoize a partial result")
}

// TestGinParamsMemoizesTheRouteParameters pins what the memo is for: the
// parameter map is built once and every later construction of one request
// reads that same map instead of allocating another.
func TestGinParamsMemoizesTheRouteParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var ran bool
	router := gin.New()
	router.GET("/api/records/:id", func(ctx *gin.Context) {
		ran = true
		ctx.Set(consts.PARAMS, []string{"id"})

		params := ginParams(ctx)
		require.Equal(t, map[string]string{"id": "42"}, params)
		require.Equal(t, reflect.ValueOf(params).Pointer(), reflect.ValueOf(ginParams(ctx)).Pointer(),
			"every construction of one request must share one map")

		// Rewriting what the map was built from proves the second call reuses
		// the stored map rather than building another.
		ctx.Params = gin.Params{{Key: "id", Value: "99"}}
		require.Equal(t, map[string]string{"id": "42"}, ginParams(ctx))
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/records/42", nil))
	require.True(t, ran, "the assertions live in the handler, so it must have run")
}

// TestGinParamsMemoizesTheAbsenceOfParameters pins the common case: a route
// declaring no parameters stores a nil map, which keeps a map out of its
// requests and still spares the later constructions the lookup that proved the
// route has none.
func TestGinParamsMemoizesTheAbsenceOfParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var ran bool
	router := gin.New()
	router.GET("/api/records", func(ctx *gin.Context) {
		ran = true
		require.Nil(t, ginParams(ctx))

		cached, stored := ctx.Get(ginParamsKey)
		require.True(t, stored, "the absence of parameters must be memoized too")
		require.Nil(t, cached)
		require.Nil(t, ginParams(ctx))
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/records", nil))
	require.True(t, ran, "the assertions live in the handler, so it must have run")
}

// TestFromGinSharesTheMemoizedParams pins the reason the memo exists at all:
// Metadata is constructed several times per request, and building the map in
// each construction cost one allocation per construction.
func TestFromGinSharesTheMemoizedParams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var ran bool
	router := gin.New()
	router.GET("/api/records/:id", func(ctx *gin.Context) {
		ran = true
		ctx.Set(consts.PARAMS, []string{"id"})

		first, second := FromGin(ctx), FromGin(ctx)
		require.Equal(t, "42", first.Param("id"))
		require.Equal(t, "42", second.Param("id"))
		require.Equal(t, reflect.ValueOf(first.params).Pointer(), reflect.ValueOf(second.params).Pointer(),
			"every construction of one request must share one map")
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/records/42", nil))
	require.True(t, ran, "the assertions live in the handler, so it must have run")
}

// TestGinClientIPMemoizesTheResolvedAddress pins what the memo is for: gin
// resolves the address once, forwarding headers included, and every later call
// of one request reads that answer instead of resolving it again.
func TestGinClientIPMemoizesTheResolvedAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginCtx.Request = httptest.NewRequest(http.MethodGet, "/api/records", nil)
	ginCtx.Request.RemoteAddr = "192.0.2.10:54321"
	ginCtx.Request.Header.Set("X-Forwarded-For", "203.0.113.5")

	require.Equal(t, "203.0.113.5", GinClientIP(ginCtx),
		"the resolution must be gin's own, forwarding headers included")

	stored, ok := ginCtx.Get(ginClientIPKey)
	require.True(t, ok, "the resolved address must be memoized for the rest of the request")
	require.Equal(t, "203.0.113.5", stored)

	// Rewriting what the address was resolved from proves the second call
	// reuses the stored answer rather than resolving again.
	ginCtx.Request.Header.Set("X-Forwarded-For", "198.51.100.7")
	require.Equal(t, "203.0.113.5", GinClientIP(ginCtx))
}

// TestGinClientIPWithoutRequestMemoizesNothing pins the construction paths that
// have no request behind them: they read an empty address and leave the memo
// untouched, so a later call that does have a request still resolves one.
func TestGinClientIPWithoutRequestMemoizesNothing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	require.Empty(t, GinClientIP(nil))

	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.Empty(t, GinClientIP(ginCtx))
	_, stored := ginCtx.Get(ginClientIPKey)
	require.False(t, stored, "without a request there is nothing to memoize")

	ginCtx.Request = httptest.NewRequest(http.MethodGet, "/api/records", nil)
	ginCtx.Request.RemoteAddr = "192.0.2.10:54321"
	require.Equal(t, "192.0.2.10", GinClientIP(ginCtx))
}

func TestMetadataContextRoundTrip(t *testing.T) {
	meta := New(Fields{
		Route:    "/api/users/:id",
		Path:     "/api/users/42",
		Username: "admin",
		UserID:   "user-1",
		TenantID: "tenant-1",
		Params: map[string]string{
			"id": "42",
		},
		Query: map[string][]string{
			"tag": {"blue", "green"},
		},
		RawQuery: "tag=blue&tag=green",
	})

	ctx := WithMetadata(context.Background(), meta)
	got := FromContext(ctx)

	require.Equal(t, "/api/users/:id", got.Route())
	require.Equal(t, "/api/users/42", got.Path())
	require.Equal(t, "admin", got.Username())
	require.Equal(t, "user-1", got.UserID())
	require.Equal(t, "tenant-1", got.TenantID())
	require.Equal(t, "42", got.Param("id"))
	require.Equal(t, []string{"blue", "green"}, got.Query()["tag"])
	require.Equal(t, "tag=blue&tag=green", got.RawQuery())
}
