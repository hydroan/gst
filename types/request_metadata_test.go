package types

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

func TestNewRequestMetadataExtractsRequestFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/users/42?tag=blue&tag=green", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "42"}}
	ctx.Set(consts.PARAMS, []string{"id"})
	ctx.Set(consts.CTX_ROUTE, "/api/users/:id")
	ctx.Set(consts.CTX_USERNAME, "admin")
	ctx.Set(consts.CTX_USER_ID, "user-1")
	ctx.Set(consts.CTX_SESSION_ID, "session-1")
	ctx.Set(consts.TRACE_ID, "trace-1")

	meta := NewRequestMetadata(ctx)

	require.Equal(t, "/api/users/:id", meta.Route())
	require.Equal(t, "admin", meta.Username())
	require.Equal(t, "user-1", meta.UserID())
	require.Equal(t, "session-1", meta.SessionID())
	require.Equal(t, "trace-1", meta.TraceID())
	require.Equal(t, "42", meta.Param("id"))
	require.Equal(t, []string{"blue", "green"}, meta.Query()["tag"])
}

func TestRequestMetadataProtectsParamsAndQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/users/42?tag=blue", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "42"}}
	ctx.Set(consts.PARAMS, []string{"id"})

	meta := NewRequestMetadata(ctx)

	params := meta.Params()
	params["id"] = "mutated"
	query := meta.Query()
	query["tag"][0] = "mutated"

	require.Equal(t, "42", meta.Param("id"))
	require.Equal(t, []string{"blue"}, meta.Query()["tag"])
}

func TestRequestMetadataContextRoundTrip(t *testing.T) {
	meta := NewRequestMetadataFromValues(RequestMetadataValues{
		Route:    "/api/users/:id",
		Username: "admin",
		UserID:   "user-1",
		TraceID:  "trace-1",
		Params: map[string]string{
			"id": "42",
		},
		Query: map[string][]string{
			"tag": {"blue", "green"},
		},
	})

	ctx := ContextWithRequestMetadata(context.Background(), meta)
	got := RequestMetadataFromContext(ctx)

	require.Equal(t, "/api/users/:id", got.Route())
	require.Equal(t, "admin", got.Username())
	require.Equal(t, "user-1", got.UserID())
	require.Equal(t, "trace-1", got.TraceID())
	require.Equal(t, "42", got.Param("id"))
	require.Equal(t, []string{"blue", "green"}, got.Query()["tag"])
}

func TestServiceContextImplementsContextAndCarriesMetadata(t *testing.T) {
	var _ context.Context = (*ServiceContext)(nil)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/users/42?tag=blue", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "42"}}
	ctx.Set(consts.PARAMS, []string{"id"})
	ctx.Set(consts.CTX_ROUTE, "/api/users/:id")
	ctx.Set(consts.CTX_USERNAME, "admin")
	ctx.Set(consts.CTX_USER_ID, "user-1")
	ctx.Set(consts.TRACE_ID, "trace-1")

	serviceCtx := NewServiceContext(ctx)
	meta := RequestMetadataFromContext(serviceCtx)

	require.Equal(t, "admin", serviceCtx.Username)
	require.Equal(t, "user-1", serviceCtx.UserID)
	require.Equal(t, "trace-1", meta.TraceID())
	require.Equal(t, "42", meta.Param("id"))
	require.Equal(t, []string{"blue"}, meta.Query()["tag"])
}

func TestModelContextImplementsContextAndCarriesMetadata(t *testing.T) {
	var _ context.Context = (*ModelContext)(nil)

	meta := NewRequestMetadataFromValues(RequestMetadataValues{
		Username: "admin",
		UserID:   "user-1",
		TraceID:  "trace-1",
	})
	ctx := ContextWithRequestMetadata(context.Background(), meta)

	modelCtx := NewModelContext(ctx)
	got := RequestMetadataFromContext(modelCtx)

	require.Equal(t, "admin", got.Username())
	require.Equal(t, "user-1", got.UserID())
	require.Equal(t, "trace-1", got.TraceID())
}
