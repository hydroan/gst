package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
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
	logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{
		Enabled:     false,
		LogRequest:  true,
		LogResponse: true,
		MaxBodySize: "64KB",
	})

	router := gin.New()
	router.Use(BodyLogger())
	router.POST("/api/logreqrsp/calc", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"a":1,"b":2}`, string(body))
		c.JSON(http.StatusOK, gin.H{"sum": 3})
	})

	w := performHTTPBodyLoggerRequest(router, "/api/logreqrsp/calc", `{"a":1,"b":2}`, "application/json")

	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, logs.All())
}

func TestHTTPBodyLoggerLogsJSONRequestAndResponse(t *testing.T) {
	logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{
		Enabled:     true,
		LogRequest:  true,
		LogResponse: true,
		MaxBodySize: "64KB",
	})

	router := gin.New()
	router.Use(BodyLogger())
	router.POST("/api/logreqrsp/calc", func(c *gin.Context) {
		c.Set(consts.CTX_USERNAME, "alice")
		c.Set(consts.CTX_USER_ID, "u-1")
		c.Set(consts.TRACE_ID, "trace-1")
		c.JSON(http.StatusOK, gin.H{
			"code":     0,
			"msg":      "success",
			"data":     gin.H{"sum": 3},
			"trace_id": "trace-1",
		})
	})

	w := performHTTPBodyLoggerRequest(router, "/api/logreqrsp/calc?verbose=true", `{"a":1,"b":2}`, "application/json")

	require.Equal(t, http.StatusOK, w.Code)
	entries := logs.All()
	require.Len(t, entries, 2)

	requestCtx := entries[0].ContextMap()
	require.Equal(t, "request", entries[0].Message)
	require.Equal(t, "/api/logreqrsp/calc", requestCtx["route"])
	require.Equal(t, "POST", requestCtx["method"])
	require.Equal(t, "alice", requestCtx["username"])
	require.Equal(t, "u-1", requestCtx["user_id"])
	require.Equal(t, "trace-1", requestCtx["trace_id"])
	require.Equal(t, map[string]any{"a": float64(1), "b": float64(2)}, requestCtx["request"])

	responseCtx := entries[1].ContextMap()
	require.Equal(t, "response", entries[1].Message)
	require.Equal(t, int64(http.StatusOK), responseCtx["status"])
	require.Equal(t, map[string]any{
		"code":     float64(0),
		"msg":      "success",
		"data":     map[string]any{"sum": float64(3)},
		"trace_id": "trace-1",
	}, responseCtx["response"])
}

func TestHTTPBodyLoggerSkipsLargeRequestBody(t *testing.T) {
	logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{
		Enabled:     true,
		LogRequest:  true,
		LogResponse: false,
		MaxBodySize: "4B",
	})

	router := gin.New()
	router.Use(BodyLogger())
	router.POST("/api/logreqrsp/calc", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"a":1}`, string(body))
		c.Status(http.StatusNoContent)
	})

	w := performHTTPBodyLoggerRequest(router, "/api/logreqrsp/calc", `{"a":1}`, "application/json")

	require.Equal(t, http.StatusNoContent, w.Code)
	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "request body too large", entries[0].Message)
	require.NotContains(t, entries[0].ContextMap(), "request")
}

func TestHTTPBodyLoggerSkipsLargeResponseBody(t *testing.T) {
	logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{
		Enabled:     true,
		LogRequest:  false,
		LogResponse: true,
		MaxBodySize: "4B",
	})

	router := gin.New()
	router.Use(BodyLogger())
	router.GET("/api/logreqrsp/calc", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"sum": 3})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logreqrsp/calc", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "response body too large", entries[0].Message)
	require.NotContains(t, entries[0].ContextMap(), "response")
}

func TestHTTPBodyLoggerIgnoresNonJSONBodies(t *testing.T) {
	logs := setupHTTPBodyLoggerTest(t, config.HTTPBodyLogger{
		Enabled:     true,
		LogRequest:  true,
		LogResponse: true,
		MaxBodySize: "64KB",
	})

	router := gin.New()
	router.Use(BodyLogger())
	router.POST("/api/logreqrsp/calc", func(c *gin.Context) {
		c.String(http.StatusOK, "sum=3")
	})

	w := performHTTPBodyLoggerRequest(router, "/api/logreqrsp/calc", "a=1&b=2", "application/x-www-form-urlencoded")

	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, logs.All())
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
	originalLogger := logger.Gin
	logger.Gin = zap.New(core)
	t.Cleanup(func() {
		logger.Gin = originalLogger
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
