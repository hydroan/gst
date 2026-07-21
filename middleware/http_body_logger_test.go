package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestHTTPBodyLoggerDisabledDoesNothing(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.HTTPBodyLogger
	}{
		{
			name: "disabled",
			cfg: config.HTTPBodyLogger{
				Enabled:     false,
				LogRequest:  config.HTTPBodyLogModeAll,
				LogResponse: config.HTTPBodyLogModeAll,
				MaxBodySize: "64KB",
			},
		},
		{
			name: "both modes none",
			cfg: config.HTTPBodyLogger{
				Enabled:     true,
				LogRequest:  config.HTTPBodyLogModeNone,
				LogResponse: config.HTTPBodyLogModeNone,
				MaxBodySize: "64KB",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := setupHTTPBodyLoggerTest(t, tc.cfg)

			router := gin.New()
			router.Use(BodyLogger())
			router.POST("/api/records", func(c *gin.Context) {
				body, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				require.JSONEq(t, `{"a":1,"b":2}`, string(body))
				c.JSON(http.StatusOK, gin.H{"sum": 3})
			})

			w := performHTTPBodyLoggerRequest(router, "/api/records", `{"a":1,"b":2}`, "application/json")

			require.Equal(t, http.StatusOK, w.Code)
			require.Empty(t, logs.All())
		})
	}
}

func TestHTTPBodyLoggerLogsRequestAndResponseAsOneEntry(t *testing.T) {
	logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{
		Enabled:     true,
		LogRequest:  config.HTTPBodyLogModeAll,
		LogResponse: config.HTTPBodyLogModeAll,
		MaxBodySize: "64KB",
	})

	router := gin.New()
	router.Use(BodyLogger())
	router.POST("/api/records", func(c *gin.Context) {
		c.Set(consts.CTX_USERNAME, "alice")
		c.Set(consts.CTX_USER_ID, "u-1")
		c.Set(consts.TRACE_ID, "trace-1")
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"a":1,"b":2}`, string(body))
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "success", "data": gin.H{"sum": 3}})
	})

	w := performHTTPBodyLoggerRequest(router, "/api/records?verbose=true", `{"a":1,"b":2}`, "application/json")

	require.Equal(t, http.StatusOK, w.Code)
	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "http_body", entries[0].Message)

	ctx := entries[0].ContextMap()
	require.Equal(t, "/api/records", ctx["route"])
	require.Equal(t, "POST", ctx["method"])
	require.Equal(t, "alice", ctx["username"])
	require.Equal(t, "u-1", ctx["user_id"])
	require.Equal(t, "trace-1", ctx["trace_id"])
	require.Equal(t, url.Values{"verbose": {"true"}}, ctx["query"])
	require.Equal(t, int64(http.StatusOK), ctx["status"])
	require.Equal(t, `{"a":1,"b":2}`, ctx["request"])
	require.Equal(t, int64(len(`{"a":1,"b":2}`)), ctx["request_size"])
	require.JSONEq(t, `{"code":0,"msg":"success","data":{"sum":3}}`, httpBodyLogStringField(t, ctx, "response"))
	require.Equal(t, int64(w.Body.Len()), ctx["response_size"])
	require.NotContains(t, ctx, "request_truncated")
	require.NotContains(t, ctx, "response_truncated")
}

func TestHTTPBodyLoggerDefaultModesLogRequestAlwaysResponseOnError(t *testing.T) {
	// The zero-value modes must fall back to "all" for requests and "error"
	// for responses.
	newRouter := func(handler gin.HandlerFunc) *gin.Engine {
		router := gin.New()
		router.Use(BodyLogger())
		router.POST("/api/records", handler)
		return router
	}

	t.Run("success omits response", func(t *testing.T) {
		logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{Enabled: true, MaxBodySize: "64KB"})
		router := newRouter(func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "success"})
		})

		w := performHTTPBodyLoggerRequest(router, "/api/records", `{"a":1}`, "application/json")

		require.Equal(t, http.StatusOK, w.Code)
		entries := logs.All()
		require.Len(t, entries, 1)
		ctx := entries[0].ContextMap()
		require.Equal(t, `{"a":1}`, ctx["request"])
		require.NotContains(t, ctx, "response")
	})

	t.Run("http error logs response", func(t *testing.T) {
		logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{Enabled: true, MaxBodySize: "64KB"})
		router := newRouter(func(c *gin.Context) {
			c.JSON(http.StatusBadRequest, gin.H{"code": -1, "msg": "failure"})
		})

		w := performHTTPBodyLoggerRequest(router, "/api/records", `{"a":1}`, "application/json")

		require.Equal(t, http.StatusBadRequest, w.Code)
		entries := logs.All()
		require.Len(t, entries, 1)
		ctx := entries[0].ContextMap()
		require.Equal(t, int64(http.StatusBadRequest), ctx["status"])
		require.Equal(t, `{"a":1}`, ctx["request"])
		require.JSONEq(t, `{"code":-1,"msg":"failure"}`, httpBodyLogStringField(t, ctx, "response"))
	})

	t.Run("envelope code marks error", func(t *testing.T) {
		logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{Enabled: true, MaxBodySize: "64KB"})
		router := newRouter(func(c *gin.Context) {
			// The response helpers record the envelope code in the gin context
			// so custom coders mapped to 2xx statuses still count as errors.
			c.Set(consts.CTX_RESPONSE_CODE, 1000)
			c.JSON(http.StatusOK, gin.H{"code": 1000, "msg": "invalid parameters"})
		})

		w := performHTTPBodyLoggerRequest(router, "/api/records", `{"a":1}`, "application/json")

		require.Equal(t, http.StatusOK, w.Code)
		entries := logs.All()
		require.Len(t, entries, 1)
		ctx := entries[0].ContextMap()
		require.Equal(t, int64(1000), ctx["code"])
		require.JSONEq(t, `{"code":1000,"msg":"invalid parameters"}`, httpBodyLogStringField(t, ctx, "response"))
	})
}

func TestHTTPBodyLoggerSkipsConfiguredRoutes(t *testing.T) {
	cfg := config.HTTPBodyLogger{
		Enabled:     true,
		LogRequest:  config.HTTPBodyLogModeAll,
		LogResponse: config.HTTPBodyLogModeAll,
		MaxBodySize: "64KB",
		SkipRoutes:  []string{"/api/login", "/api/records/*"},
	}
	newRouter := func() *gin.Engine {
		router := gin.New()
		router.Use(BodyLogger())
		handler := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }
		router.POST("/api/login", handler)
		router.POST("/api/records/:id/notes", handler)
		router.POST("/api/items", handler)
		return router
	}

	t.Run("exact match", func(t *testing.T) {
		logs := setupHTTPBodyLoggerTest(t, cfg)
		w := performHTTPBodyLoggerRequest(newRouter(), "/api/login", `{"username":"alice"}`, "application/json")
		require.Equal(t, http.StatusOK, w.Code)
		require.Empty(t, logs.All())
	})

	t.Run("prefix wildcard", func(t *testing.T) {
		logs := setupHTTPBodyLoggerTest(t, cfg)
		w := performHTTPBodyLoggerRequest(newRouter(), "/api/records/1/notes", `{"a":1}`, "application/json")
		require.Equal(t, http.StatusOK, w.Code)
		require.Empty(t, logs.All())
	})

	t.Run("other routes still logged", func(t *testing.T) {
		logs := setupHTTPBodyLoggerTest(t, cfg)
		w := performHTTPBodyLoggerRequest(newRouter(), "/api/items", `{"a":1}`, "application/json")
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, logs.All(), 1)
	})
}

func TestHTTPBodyLoggerSkipsLargeRequestBodyContent(t *testing.T) {
	logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{
		Enabled:     true,
		LogRequest:  config.HTTPBodyLogModeAll,
		LogResponse: config.HTTPBodyLogModeNone,
		MaxBodySize: "4B",
	})

	router := gin.New()
	router.Use(BodyLogger())
	router.POST("/api/records", func(c *gin.Context) {
		// The oversized body is not buffered by the middleware, so the
		// handler must still receive the original stream untouched.
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"a":1}`, string(body))
		c.Status(http.StatusNoContent)
	})

	w := performHTTPBodyLoggerRequest(router, "/api/records", `{"a":1}`, "application/json")

	require.Equal(t, http.StatusNoContent, w.Code)
	entries := logs.All()
	require.Len(t, entries, 1)
	ctx := entries[0].ContextMap()
	require.NotContains(t, ctx, "request")
	require.Equal(t, true, ctx["request_truncated"])
	require.Equal(t, int64(len(`{"a":1}`)), ctx["request_size"])
}

func TestHTTPBodyLoggerTruncatesLargeResponseBody(t *testing.T) {
	logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{
		Enabled:     true,
		LogRequest:  config.HTTPBodyLogModeNone,
		LogResponse: config.HTTPBodyLogModeAll,
		MaxBodySize: "4B",
	})

	router := gin.New()
	router.Use(BodyLogger())
	router.GET("/api/records", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"sum": 3})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/records", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	entries := logs.All()
	require.Len(t, entries, 1)
	ctx := entries[0].ContextMap()
	require.Equal(t, `{"su`, ctx["response"])
	require.Equal(t, true, ctx["response_truncated"])
	require.Equal(t, int64(w.Body.Len()), ctx["response_size"])
}

func TestHTTPBodyLoggerIgnoresNonJSONBodies(t *testing.T) {
	logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{
		Enabled:     true,
		LogRequest:  config.HTTPBodyLogModeAll,
		LogResponse: config.HTTPBodyLogModeAll,
		MaxBodySize: "64KB",
	})

	router := gin.New()
	router.Use(BodyLogger())
	router.POST("/api/records", func(c *gin.Context) {
		c.String(http.StatusOK, "sum=3")
	})

	w := performHTTPBodyLoggerRequest(router, "/api/records", "a=1&b=2", "application/x-www-form-urlencoded")

	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, logs.All())
}

func TestHTTPBodyLoggerLogsMalformedJSONVerbatim(t *testing.T) {
	logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{
		Enabled:     true,
		LogRequest:  config.HTTPBodyLogModeAll,
		LogResponse: config.HTTPBodyLogModeNone,
		MaxBodySize: "64KB",
	})

	router := gin.New()
	router.Use(BodyLogger())
	router.POST("/api/records", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Equal(t, `{"a":`, string(body))
		c.Status(http.StatusBadRequest)
	})

	w := performHTTPBodyLoggerRequest(router, "/api/records", `{"a":`, "application/json")

	require.Equal(t, http.StatusBadRequest, w.Code)
	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, `{"a":`, entries[0].ContextMap()["request"])
}

func httpBodyLogStringField(t *testing.T, ctx map[string]any, key string) string {
	t.Helper()

	value, ok := ctx[key].(string)
	require.True(t, ok, "log field %q is not a string", key)
	return value
}

func setupHTTPBodyLoggerTest(t *testing.T, cfg config.HTTPBodyLogger) *observer.ObservedLogs {
	t.Helper()

	gin.SetMode(gin.TestMode)

	originalConfig := config.App
	config.App = new(config.Config)
	config.App.Logger.HTTPBody = cfg
	t.Cleanup(func() {
		config.App = originalConfig
	})

	core, logs := observer.New(zapcore.InfoLevel)
	originalLogger := logger.HTTPBody
	logger.HTTPBody = zap.New(core)
	t.Cleanup(func() {
		logger.HTTPBody = originalLogger
	})

	return logs
}

func performHTTPBodyLoggerRequest(router *gin.Engine, path, body, contentType string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", contentType)
	router.ServeHTTP(w, req)
	return w
}
