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
		ctx.Set(consts.TRACE_ID, "trace-1")

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
	require.Equal(t, "trace-1", meta.TraceID())
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
	require.Equal(t, reflect.ValueOf(values).Pointer(), reflect.ValueOf(GinQueryValues(ginCtx)).Pointer(),
		"every construction of one request must share one parse")
}

func TestMetadataContextRoundTrip(t *testing.T) {
	meta := New(Fields{
		Route:    "/api/users/:id",
		Path:     "/api/users/42",
		Username: "admin",
		UserID:   "user-1",
		TenantID: "tenant-1",
		TraceID:  "trace-1",
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
	require.Equal(t, "trace-1", got.TraceID())
	require.Equal(t, "42", got.Param("id"))
	require.Equal(t, []string{"blue", "green"}, got.Query()["tag"])
	require.Equal(t, "tag=blue&tag=green", got.RawQuery())
}
