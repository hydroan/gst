package types

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

func TestServiceContextContextMethods(t *testing.T) {
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
	ctx.Set(consts.CTX_TENANT_ID, "tenant-1")
	ctx.Set(consts.TRACE_ID, "trace-1")

	serviceCtx := NewServiceContext(ctx, nil, "")
	meta := requestctx.FromContext(serviceCtx)

	require.Equal(t, "admin", serviceCtx.Username())
	require.Equal(t, "user-1", serviceCtx.UserID())
	require.Equal(t, "tenant-1", serviceCtx.TenantID())
	require.Equal(t, "trace-1", meta.TraceID())
	require.Equal(t, "42", meta.Param("id"))
	require.Equal(t, []string{"blue"}, meta.Query()["tag"])
}

func TestServiceContextQueryAccessorReturnsCopy(t *testing.T) {
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

	serviceCtx := NewServiceContext(ctx, nil, "")

	query := serviceCtx.Query()
	query["tag"][0] = "mutated"

	require.Equal(t, "42", serviceCtx.Param("id"))
	require.Equal(t, []string{"blue"}, serviceCtx.Query()["tag"])
	require.Equal(t, "/api/users/:id", serviceCtx.Route())
	require.Equal(t, "trace-1", serviceCtx.TraceID())
}

func TestNewServiceContextStoresPhase(t *testing.T) {
	serviceCtx := NewServiceContext(nil, nil, consts.PHASE_LIST)

	require.Equal(t, consts.PHASE_LIST, serviceCtx.Phase())
}

func TestServiceContextRequestAccessors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "https://example.com/api/users?tag=blue", nil)

	serviceCtx := NewServiceContext(ctx, nil, "")

	require.Equal(t, http.MethodGet, serviceCtx.Method())
	require.Equal(t, "/api/users", serviceCtx.Path())
	require.Equal(t, "example.com", serviceCtx.Host())
	require.True(t, serviceCtx.IsHTTPS())
	require.Equal(t, "blue", serviceCtx.Query().Get("tag"))
}

func TestServiceContextNilRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	serviceCtx := NewServiceContext(ctx, nil, "")

	require.Empty(t, serviceCtx.Method())
	require.Empty(t, serviceCtx.Path())
	require.Empty(t, serviceCtx.Host())
	require.Empty(t, serviceCtx.ClientIP())
	require.Empty(t, serviceCtx.UserAgent())
	require.False(t, serviceCtx.IsHTTPS())
	require.Empty(t, serviceCtx.Query())
}

func TestServiceContextResponseHelpers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "https://example.com/api/users?tag=blue", nil)

	serviceCtx := NewServiceContext(ctx, nil, "")
	serviceCtx.SetCookie(&http.Cookie{
		Name:     "session_id",
		Value:    "session-1",
		Path:     "/",
		HttpOnly: true,
		Secure:   serviceCtx.IsHTTPS(),
		SameSite: http.SameSiteLaxMode,
	})
	serviceCtx.Data(http.StatusCreated, "text/plain", []byte("created"))

	setCookie := recorder.Header().Get("Set-Cookie")
	require.Contains(t, setCookie, "session_id=session-1")
	require.Contains(t, setCookie, "Path=/")
	require.Contains(t, setCookie, "HttpOnly")
	require.Contains(t, setCookie, "Secure")
	require.Contains(t, setCookie, "SameSite=Lax")
	require.Equal(t, http.StatusCreated, recorder.Code)
	require.Equal(t, "created", recorder.Body.String())
}

func TestServiceContextNilGinHelpers(t *testing.T) {
	serviceCtx := NewServiceContext(nil, nil, "")

	serviceCtx.Data(http.StatusCreated, "text/plain", []byte("created"))
	serviceCtx.SetCookie(&http.Cookie{Name: "session_id", Value: "session-1"})

	require.Empty(t, serviceCtx.PostForm("name"))

	cookie, err := serviceCtx.Cookie("session_id")
	require.Error(t, err)
	require.Empty(t, cookie)

	file, err := serviceCtx.FormFile("file")
	require.Error(t, err)
	require.Nil(t, file)

	var nilCtx *ServiceContext
	nilCtx.Data(http.StatusCreated, "text/plain", []byte("created"))
	nilCtx.SetCookie(&http.Cookie{Name: "session_id", Value: "session-1"})
	require.Empty(t, nilCtx.PostForm("name"))

	cookie, err = nilCtx.Cookie("session_id")
	require.Error(t, err)
	require.Empty(t, cookie)

	file, err = nilCtx.FormFile("file")
	require.Error(t, err)
	require.Nil(t, file)
}
