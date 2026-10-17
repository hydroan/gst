package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hydroan/gst/internal/types"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestRecoveryWithTracingAnswersInTheEnvelope pins that a recovered panic is
// answered in the API envelope.
//
// Aborting with a bare 500 and no body at all would leave a client reading the
// documented shape unable to tell the refusal from a malformed response, and
// the one answer whose reader most needs the trace id that explains it would
// carry none.
func TestRecoveryWithTracingAnswersInTheEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(recoveryWithTracing(nil, false))
	engine.GET("/panic", func(*gin.Context) { panic("boom") })

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")

	var envelope struct {
		Msg     string           `json:"msg"`
		Data    *json.RawMessage `json:"data"`
		TraceID *string          `json:"trace_id"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope),
		"response body: %s", recorder.Body.String())
	require.Equal(t, types.FailureMsg, envelope.Msg)
	require.NotNil(t, envelope.TraceID, "a refusal has to carry the trace that explains it")
}

// TestRecoveryLeavesAStartedResponseAlone pins what a panic does to a
// response that has begun, a stream whose headers and first events are out:
// nothing is appended to it, since the envelope would be read as part of the
// stream, and the panic is reported on the context instead, for the access
// log and the span to carry.
func TestRecoveryLeavesAStartedResponseAlone(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var reported []error
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Next()
		for _, err := range c.Errors {
			reported = append(reported, err.Err)
		}
	})
	engine.Use(recoveryWithTracing(nil, false))
	engine.GET("/events", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Status(http.StatusOK)
		_, _ = c.Writer.WriteString("data: one\n\n")
		c.Writer.Flush()
		panic("boom")
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "data: one\n\n", recorder.Body.String(), "a response that has begun must end where it is")
	require.Len(t, reported, 1)
	require.ErrorContains(t, reported[0], "boom")
}

// TestRecoveryLogsThePanicToTheRecoveryLogger pins where the chain's
// recovery writes: logger.Recovery, the logger the gRPC listener's panics go
// to as well, with the panic and its stack in the message and the trace id
// of the request as a field, so one search of the file finds the panic that
// explains a response.
func TestRecoveryLogsThePanicToTheRecoveryLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zapcore.DebugLevel)
	saved := logger.Recovery
	logger.Recovery = zap.New(core)
	t.Cleanup(func() { logger.Recovery = saved })

	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set(consts.TRACE_ID, "trace-panic") }, recovery())
	engine.GET("/panic", func(*gin.Context) { panic("boom") })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	entries := logs.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "panic recovered")
	require.Contains(t, entries[0].Message, "boom")
	require.Contains(t, entries[0].Message, "GET /panic")
	require.Contains(t, entries[0].Message, "goroutine ", "the stack of the panic")
	require.Equal(t, "trace-panic", entries[0].ContextMap()[consts.TRACE_ID])
}
