package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/response"
	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/require"
)

// TestCircuitBreakerLeavesTheAnswerOfARequestItLetThrough pins that the
// breaker answers only the requests it refuses. A request it let through that
// then failed is counted against the breaker, and its answer stays the one the
// handlers gave: a server error the handler wrote is not followed by a second
// envelope, and a failure the handler only recorded is not turned into a 503
// while the breaker is still closed.
func TestCircuitBreakerLeavesTheAnswerOfARequestItLetThrough(t *testing.T) {
	t.Run("a handler that answered with a server error", func(t *testing.T) {
		withClosedBreaker(t)

		w := serveThroughBreaker(func(c *gin.Context) {
			response.Abort(c, http.StatusInternalServerError, "sample failure")
		})

		require.Equal(t, http.StatusInternalServerError, w.Code)
		var envelope struct {
			Msg string `json:"msg"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope), "the body must be the handler's one envelope: %s", w.Body.String())
		require.Equal(t, "sample failure", envelope.Msg)
		require.Equal(t, uint32(1), cb.Counts().TotalFailures, "the failure must still count against the breaker")
	})

	t.Run("a handler that recorded an error without answering", func(t *testing.T) {
		withClosedBreaker(t)

		w := serveThroughBreaker(func(c *gin.Context) {
			_ = c.Error(errors.New("sample failure"))
		})

		require.Equal(t, http.StatusOK, w.Code, "the answer stays the one gin gives a request nobody answered")
		require.Empty(t, w.Body.String())
		require.Equal(t, uint32(1), cb.Counts().TotalFailures, "the failure must still count against the breaker")
	})
}

// withClosedBreaker swaps in, for one test, a breaker that stays closed
// through the one failure a test sends it.
func withClosedBreaker(t *testing.T) {
	t.Helper()

	original := cb
	t.Cleanup(func() { cb = original })
	cb = gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "closed-test",
		Timeout:     time.Hour,
		ReadyToTrip: func(gobreaker.Counts) bool { return false },
	})
}

// serveThroughBreaker sends one request through the circuit breaker to handle
// and returns the recorded response.
func serveThroughBreaker(handle gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CircuitBreaker())
	router.GET("/api/samples", handle)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/samples", nil))
	return w
}
