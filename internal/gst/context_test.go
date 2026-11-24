package gst_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/sse"
	"github.com/stretchr/testify/require"
)

func TestServiceContextCarriesRequestMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var serviceCtx *gst.ServiceContext
	router := gin.New()
	router.GET("/api/users/:id", func(ctx *gin.Context) {
		ctx.Set(requestctx.KeyParams, []string{"id"})
		ctx.Set(requestctx.KeyUsername, "admin")
		ctx.Set(requestctx.KeyUserID, "user-1")
		ctx.Set(requestctx.KeyTenantID, "tenant-1")
		ctx.Set(requestctx.KeyRequiresAuth, true)
		ctx.Request = ctx.Request.WithContext(execctx.WithTraceID(ctx.Request.Context(), "trace-1"))

		serviceCtx = gst.NewServiceContext(ctx, nil, "")
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

	var serviceCtx *gst.ServiceContext
	router := gin.New()
	router.GET("/api/users/:id", func(ctx *gin.Context) {
		ctx.Set(requestctx.KeyParams, []string{"id"})
		ctx.Set(requestctx.KeyUsername, "admin")
		ctx.Set(requestctx.KeyUserID, "user-1")
		ctx.Request = ctx.Request.WithContext(execctx.WithTraceID(ctx.Request.Context(), "trace-1"))

		serviceCtx = gst.NewServiceContext(ctx, nil, "")
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
	serviceCtx := gst.NewServiceContext(nil, nil, apiaction.List)

	require.Equal(t, apiaction.List, serviceCtx.Phase())
}

func TestServiceContextRequestAccessors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "https://example.com/api/users?tag=blue", nil)
	ctx.Request.Header.Set("User-Agent", "sample-agent/1.0")

	serviceCtx := gst.NewServiceContext(ctx, nil, "")

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

	serviceCtx := gst.NewServiceContext(ctx, nil, "")

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

	serviceCtx := gst.NewServiceContext(nil, ctx, apiaction.Get)

	require.Equal(t, apiaction.Get, serviceCtx.Phase())
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

	serviceCtx := gst.NewServiceContext(ctx, nil, "")
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
	require.False(t, gst.HTTPOnlyMethodCalled(serviceCtx), "the writes reached the HTTP response")
}

// TestHTTPOnlyMethodCalledRecordsCallsWithoutHTTP pins the flag a transport
// other than HTTP reads once a hook or a service method returns: every
// HTTP-only method sets
// it when there is no HTTP request or response behind the context, the
// readers of the request as much as the writers of the response, and a nil
// cookie -- nothing to write on any transport -- does not.
func TestHTTPOnlyMethodCalledRecordsCallsWithoutHTTP(t *testing.T) {
	tests := []struct {
		name string
		call func(serviceCtx *gst.ServiceContext)
		want bool
	}{
		{name: "Data", call: func(serviceCtx *gst.ServiceContext) {
			serviceCtx.Data(http.StatusCreated, "text/plain", []byte("created"))
		}, want: true},
		{name: "SetCookie", call: func(serviceCtx *gst.ServiceContext) {
			serviceCtx.SetCookie(&http.Cookie{Name: "session_id", Value: "session-1"})
		}, want: true},
		{name: "SSE", call: func(serviceCtx *gst.ServiceContext) {
			_ = serviceCtx.SSE(func(*sse.Conn) error { return nil })
		}, want: true},
		{name: "SetCookie nil", call: func(serviceCtx *gst.ServiceContext) {
			serviceCtx.SetCookie(nil)
		}, want: false},
		{name: "Cookie", call: func(serviceCtx *gst.ServiceContext) {
			_, _ = serviceCtx.Cookie("session_id")
		}, want: true},
		{name: "PostForm", call: func(serviceCtx *gst.ServiceContext) {
			_ = serviceCtx.PostForm("name")
		}, want: true},
		{name: "FormFile", call: func(serviceCtx *gst.ServiceContext) {
			_, _ = serviceCtx.FormFile("file")
		}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceCtx := gst.NewServiceContext(nil, nil, "")
			require.False(t, gst.HTTPOnlyMethodCalled(serviceCtx))

			tt.call(serviceCtx)

			require.Equal(t, tt.want, gst.HTTPOnlyMethodCalled(serviceCtx))
		})
	}

	var nilCtx *gst.ServiceContext
	nilCtx.Data(http.StatusCreated, "text/plain", []byte("created"))
	require.False(t, gst.HTTPOnlyMethodCalled(nilCtx))
}

func TestServiceContextNilGinHelpers(t *testing.T) {
	serviceCtx := gst.NewServiceContext(nil, nil, "")

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

	var nilCtx *gst.ServiceContext
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
	sc := reflect.TypeFor[*gst.ServiceContext]()
	for _, name := range gst.HTTPOnlyMethods {
		if _, ok := sc.MethodByName(name); !ok {
			t.Errorf("HTTPOnlyMethods names %s, which ServiceContext does not declare", name)
		}
	}
	if !slices.IsSorted(gst.HTTPOnlyMethods) {
		t.Errorf("HTTPOnlyMethods should be sorted, got %v", gst.HTTPOnlyMethods)
	}
}

// TestServiceContextDelegatesTheDeadlineOfItsContext pins that a
// ServiceContext answers the deadline of the context it was made on, the
// way Done, Err and Value answer that context, which is what lets it stand
// wherever a context.Context is expected.
func TestServiceContextDelegatesTheDeadlineOfItsContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deadline := time.Now().Add(time.Hour)

	var serviceCtx *gst.ServiceContext
	router := gin.New()
	router.GET("/api/samples", func(ctx *gin.Context) {
		bounded, cancel := context.WithDeadline(ctx.Request.Context(), deadline)
		defer cancel()
		ctx.Request = ctx.Request.WithContext(bounded)
		serviceCtx = gst.NewServiceContext(ctx, nil, "")
		got, ok := serviceCtx.Deadline()
		require.True(t, ok)
		require.Equal(t, deadline, got)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/samples", nil))
	require.NotNil(t, serviceCtx, "the handler must have run")

	_, ok := gst.NewServiceContext(nil, nil, "").Deadline()
	require.False(t, ok, "a context made on no request has no deadline to answer")
}
