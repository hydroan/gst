package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestRecoveryWithTracingAnswersInTheEnvelope pins that a recovered panic is
// answered in the API envelope.
//
// It used to abort with a bare 500 and no body at all: a client reading the
// documented shape could not tell the refusal from a malformed response, and
// the one answer whose reader most needs the trace id that explains it carried
// none.
func TestRecoveryWithTracingAnswersInTheEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(RecoveryWithTracing(nil, false))
	engine.GET("/panic", func(*gin.Context) { panic("boom") })

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")

	var envelope struct {
		Code    *int             `json:"code"`
		Msg     string           `json:"msg"`
		Data    *json.RawMessage `json:"data"`
		TraceID *string          `json:"trace_id"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope),
		"response body: %s", recorder.Body.String())
	require.NotNil(t, envelope.Code, "a refusal has to carry a code, like every other response")
	require.Equal(t, -1, *envelope.Code)
	require.Equal(t, "internal server error", envelope.Msg)
	require.NotNil(t, envelope.TraceID, "a refusal has to carry the trace that explains it")
}
