package types_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/sse"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

func TestServiceContextCarriesRequestMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var serviceCtx *types.ServiceContext
	router := gin.New()
	router.GET("/api/users/:id", func(ctx *gin.Context) {
		ctx.Set(consts.PARAMS, []string{"id"})
		ctx.Set(consts.CTX_USERNAME, "admin")
		ctx.Set(consts.CTX_USER_ID, "user-1")
		ctx.Set(consts.CTX_TENANT_ID, "tenant-1")
		ctx.Set(consts.CTX_REQUIRES_AUTH, true)
		ctx.Request = ctx.Request.WithContext(execctx.WithTraceID(ctx.Request.Context(), "trace-1"))

		serviceCtx = types.NewServiceContext(ctx, nil, "")
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/users/42?tag=blue", nil))

	meta := requestctx.FromContext(serviceCtx)

	require.Equal(t, "admin", serviceCtx.Username())
	require.Equal(t, "user-1", serviceCtx.UserID())
	require.Equal(t, "tenant-1", serviceCtx.TenantID())
	require.Equal(t, "trace-1", serviceCtx.TraceID())
	require.True(t, serviceCtx.RequiresAuth(), "the auth marker declared the route authenticated")
	require.Equal(t, "42", meta.Param("id"))
	require.Equal(t, []string{"blue"}, meta.Query()["tag"])
}

func TestServiceContextQueryAccessorReturnsCopy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var serviceCtx *types.ServiceContext
	router := gin.New()
	router.GET("/api/users/:id", func(ctx *gin.Context) {
		ctx.Set(consts.PARAMS, []string{"id"})
		ctx.Set(consts.CTX_USERNAME, "admin")
		ctx.Set(consts.CTX_USER_ID, "user-1")
		ctx.Request = ctx.Request.WithContext(execctx.WithTraceID(ctx.Request.Context(), "trace-1"))

		serviceCtx = types.NewServiceContext(ctx, nil, "")
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/users/42?tag=blue", nil))

	query := serviceCtx.Query()
	query["tag"][0] = "mutated"

	require.Equal(t, "42", serviceCtx.Param("id"))
	require.Equal(t, []string{"blue"}, serviceCtx.Query()["tag"])
	require.Equal(t, "/api/users/:id", serviceCtx.Route())
	require.Equal(t, "/api/users/42", serviceCtx.Path())
	require.Equal(t, "trace-1", serviceCtx.TraceID())
}

func TestNewServiceContextStoresPhase(t *testing.T) {
	serviceCtx := types.NewServiceContext(nil, nil, consts.PHASE_LIST)

	require.Equal(t, consts.PHASE_LIST, serviceCtx.Phase())
}

func TestServiceContextRequestAccessors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "https://example.com/api/users?tag=blue", nil)
	ctx.Request.Header.Set("User-Agent", "sample-agent/1.0")

	serviceCtx := types.NewServiceContext(ctx, nil, "")

	require.Equal(t, http.MethodGet, serviceCtx.Method())
	require.Equal(t, "/api/users", serviceCtx.Path())
	require.Equal(t, "example.com", serviceCtx.Host())
	require.Equal(t, "192.0.2.1", serviceCtx.ClientIP())
	require.Equal(t, "sample-agent/1.0", serviceCtx.UserAgent())
	require.True(t, serviceCtx.IsHTTPS())
	require.Equal(t, "blue", serviceCtx.Query().Get("tag"))
}

func TestServiceContextNilRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	serviceCtx := types.NewServiceContext(ctx, nil, "")

	require.Empty(t, serviceCtx.Method())
	require.Empty(t, serviceCtx.Path())
	require.Empty(t, serviceCtx.Host())
	require.Empty(t, serviceCtx.ClientIP())
	require.Empty(t, serviceCtx.UserAgent())
	require.False(t, serviceCtx.IsHTTPS())
	require.Empty(t, serviceCtx.Query())
}

// TestServiceContextWithoutGinReadsMetadataFromContext pins the construction
// path a transport other than HTTP uses: with no Gin request, every request
// accessor answers from the metadata the transport attached to ctx.
func TestServiceContextWithoutGinReadsMetadataFromContext(t *testing.T) {
	ctx := requestctx.WithMetadata(context.Background(), requestctx.New(requestctx.Fields{
		Route:        "/api/users/:id",
		Path:         "/api/users/42",
		Method:       http.MethodGet,
		Username:     "admin",
		ClientIP:     "203.0.113.5",
		UserAgent:    "sample-agent/1.0",
		Host:         "example.com",
		TLS:          true,
		RequiresAuth: true,
	}))

	serviceCtx := types.NewServiceContext(nil, ctx, consts.PHASE_GET)

	require.Equal(t, consts.PHASE_GET, serviceCtx.Phase())
	require.Equal(t, "/api/users/:id", serviceCtx.Route())
	require.Equal(t, "/api/users/42", serviceCtx.Path())
	require.Equal(t, http.MethodGet, serviceCtx.Method())
	require.Equal(t, "admin", serviceCtx.Username())
	require.Equal(t, "203.0.113.5", serviceCtx.ClientIP())
	require.Equal(t, "sample-agent/1.0", serviceCtx.UserAgent())
	require.Equal(t, "example.com", serviceCtx.Host())
	require.True(t, serviceCtx.IsHTTPS())
	require.True(t, serviceCtx.RequiresAuth(), "the metadata declared the action authenticated")
}

func TestServiceContextResponseHelpers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "https://example.com/api/users?tag=blue", nil)

	serviceCtx := types.NewServiceContext(ctx, nil, "")
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
	require.False(t, types.RawResponseAttempted(serviceCtx), "the writes reached the HTTP response")
}

// TestRawResponseAttemptedRecordsWritesWithoutHTTP pins the flag a transport
// other than HTTP reads once the service returns: each response writer sets it
// when there is no HTTP response to write to, the request readers do not, and
// a nil cookie -- nothing to write on any transport -- does not either.
func TestRawResponseAttemptedRecordsWritesWithoutHTTP(t *testing.T) {
	tests := []struct {
		name string
		call func(serviceCtx *types.ServiceContext)
		want bool
	}{
		{name: "Data", call: func(serviceCtx *types.ServiceContext) {
			serviceCtx.Data(http.StatusCreated, "text/plain", []byte("created"))
		}, want: true},
		{name: "SetCookie", call: func(serviceCtx *types.ServiceContext) {
			serviceCtx.SetCookie(&http.Cookie{Name: "session_id", Value: "session-1"})
		}, want: true},
		{name: "SSE", call: func(serviceCtx *types.ServiceContext) {
			_ = serviceCtx.SSE(func(*sse.Conn) error { return nil })
		}, want: true},
		{name: "SetCookie nil", call: func(serviceCtx *types.ServiceContext) {
			serviceCtx.SetCookie(nil)
		}, want: false},
		{name: "Cookie", call: func(serviceCtx *types.ServiceContext) {
			_, _ = serviceCtx.Cookie("session_id")
		}, want: false},
		{name: "PostForm", call: func(serviceCtx *types.ServiceContext) {
			_ = serviceCtx.PostForm("name")
		}, want: false},
		{name: "FormFile", call: func(serviceCtx *types.ServiceContext) {
			_, _ = serviceCtx.FormFile("file")
		}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceCtx := types.NewServiceContext(nil, nil, "")
			require.False(t, types.RawResponseAttempted(serviceCtx))

			tt.call(serviceCtx)

			require.Equal(t, tt.want, types.RawResponseAttempted(serviceCtx))
		})
	}

	var nilCtx *types.ServiceContext
	nilCtx.Data(http.StatusCreated, "text/plain", []byte("created"))
	require.False(t, types.RawResponseAttempted(nilCtx))
}

func TestServiceContextNilGinHelpers(t *testing.T) {
	serviceCtx := types.NewServiceContext(nil, nil, "")

	serviceCtx.Data(http.StatusCreated, "text/plain", []byte("created"))
	serviceCtx.SetCookie(&http.Cookie{Name: "session_id", Value: "session-1"})
	require.Error(t, serviceCtx.SSE(func(*sse.Conn) error { return nil }))

	require.Empty(t, serviceCtx.PostForm("name"))

	cookie, err := serviceCtx.Cookie("session_id")
	require.Error(t, err)
	require.Empty(t, cookie)

	file, err := serviceCtx.FormFile("file")
	require.Error(t, err)
	require.Nil(t, file)

	var nilCtx *types.ServiceContext
	nilCtx.Data(http.StatusCreated, "text/plain", []byte("created"))
	nilCtx.SetCookie(&http.Cookie{Name: "session_id", Value: "session-1"})
	require.Error(t, nilCtx.SSE(func(*sse.Conn) error { return nil }))
	require.Empty(t, nilCtx.PostForm("name"))

	cookie, err = nilCtx.Cookie("session_id")
	require.Error(t, err)
	require.Empty(t, cookie)

	file, err = nilCtx.FormFile("file")
	require.Error(t, err)
	require.Nil(t, file)
}

// TestHTTPOnlyMethodsNameMethodsOfServiceContext pins that every name in
// HTTPOnlyMethods is a method of ServiceContext, so the list gg check reads
// cannot drift from the type.
func TestHTTPOnlyMethodsNameMethodsOfServiceContext(t *testing.T) {
	sc := reflect.TypeFor[*types.ServiceContext]()
	for _, name := range types.HTTPOnlyMethods {
		if _, ok := sc.MethodByName(name); !ok {
			t.Errorf("HTTPOnlyMethods names %s, which ServiceContext does not declare", name)
		}
	}
	if !slices.IsSorted(types.HTTPOnlyMethods) {
		t.Errorf("HTTPOnlyMethods should be sorted, got %v", types.HTTPOnlyMethods)
	}
}
