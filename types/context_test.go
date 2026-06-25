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

	require.Equal(t, "admin", serviceCtx.Username())
	require.Equal(t, "user-1", serviceCtx.UserID())
	require.Equal(t, "trace-1", meta.TraceID())
	require.Equal(t, "42", meta.Param("id"))
	require.Equal(t, []string{"blue"}, meta.Query()["tag"])
}

func TestServiceContextAccessorsReturnCopies(t *testing.T) {
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

	params := serviceCtx.Params()
	params["id"] = "mutated"
	query := serviceCtx.Query()
	query["tag"][0] = "mutated"

	require.Equal(t, "42", serviceCtx.Param("id"))
	require.Equal(t, []string{"blue"}, serviceCtx.Query()["tag"])
	require.Equal(t, "/api/users/:id", serviceCtx.Route())
	require.Equal(t, "trace-1", serviceCtx.TraceID())
}

func TestServiceContextWithPhaseReturnsClone(t *testing.T) {
	serviceCtx := NewServiceContext(nil)

	phased := serviceCtx.WithPhase(consts.PHASE_LIST)

	require.NotSame(t, serviceCtx, phased)
	require.Empty(t, serviceCtx.GetPhase())
	require.Equal(t, consts.PHASE_LIST, phased.GetPhase())
}

func TestServiceContextRequestAccessorsAndSetCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "https://example.com/api/users?tag=blue", nil)

	serviceCtx := NewServiceContext(ctx)
	serviceCtx.SetCookie(&http.Cookie{
		Name:     "session_id",
		Value:    "session-1",
		Path:     "/",
		HttpOnly: true,
		Secure:   serviceCtx.IsHTTPS(),
		SameSite: http.SameSiteLaxMode,
	})

	require.Equal(t, "example.com", serviceCtx.Host())
	require.True(t, serviceCtx.IsHTTPS())
	require.Equal(t, "blue", serviceCtx.Query().Get("tag"))

	setCookie := recorder.Header().Get("Set-Cookie")
	require.Contains(t, setCookie, "session_id=session-1")
	require.Contains(t, setCookie, "Path=/")
	require.Contains(t, setCookie, "HttpOnly")
	require.Contains(t, setCookie, "Secure")
	require.Contains(t, setCookie, "SameSite=Lax")
}
