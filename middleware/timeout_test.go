package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	internalmiddleware "github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/middleware"
	"github.com/stretchr/testify/require"
)

// timeoutUnderTest is short enough to keep the tests fast and long enough for
// a handler that answers at once to finish well inside it.
const timeoutUnderTest = 30 * time.Millisecond

func TestTimeoutLeavesStreamingRouteAlone(t *testing.T) {
	internalmiddleware.MarkStreamingRoute(http.MethodGet, "/api/timeout/stream")

	router := newTimeoutRouter()
	router.GET("/api/timeout/stream", func(c *gin.Context) {
		// Outlives the timeout on purpose; only the streaming exemption lets
		// this response finish.
		time.Sleep(4 * timeoutUnderTest)
		c.String(http.StatusOK, "finished")
	})

	stream := serveTimeout(router, "/api/timeout/stream")
	require.Equal(t, http.StatusOK, stream.Code)
	require.Equal(t, "finished", stream.Body.String())
}

func TestTimeoutPassesAResponseStartedInTime(t *testing.T) {
	router := newTimeoutRouter()
	router.GET("/api/timeout/prompt", func(c *gin.Context) {
		c.Header("X-Handler", "set")
		c.String(http.StatusCreated, "answered")
	})

	w := serveTimeout(router, "/api/timeout/prompt")
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, "answered", w.Body.String())
	require.Equal(t, "set", w.Header().Get("X-Handler"))
	require.Equal(t, "kept", w.Header().Get("X-Upstream"))
}

// TestTimeoutAnswersALateResponseWithGatewayTimeout covers a handler that
// ignores the deadline and only starts its response after it: the client gets
// the 504 envelope instead, and nothing the handler set or wrote gets through.
// The test waits for the handler before looking, so a write that lands after
// the middleware has returned would show up here, and under -race so would the
// handler touching the context the engine has already taken back.
func TestTimeoutAnswersALateResponseWithGatewayTimeout(t *testing.T) {
	router := newTimeoutRouter()
	finished := make(chan struct{})
	router.GET("/api/timeout/late", func(c *gin.Context) {
		defer close(finished)
		time.Sleep(4 * timeoutUnderTest)
		c.Header("X-Handler", "set")
		c.String(http.StatusOK, "late")
	})

	w := serveTimeout(router, "/api/timeout/late")
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler did not finish")
	}

	requireGatewayTimeout(t, w)
	require.Empty(t, w.Header().Get("X-Handler"))
	require.Equal(t, "kept", w.Header().Get("X-Upstream"))
}

// TestTimeoutAnswersAStoppedHandlerWithGatewayTimeout covers a handler that
// honors the deadline: it stops when the request context ends and writes an
// error of its own, which the 504 envelope replaces.
func TestTimeoutAnswersAStoppedHandlerWithGatewayTimeout(t *testing.T) {
	router := newTimeoutRouter()
	router.GET("/api/timeout/stopped", func(c *gin.Context) {
		<-c.Request.Context().Done()
		c.JSON(http.StatusInternalServerError, gin.H{"error": c.Request.Context().Err().Error()})
	})

	requireGatewayTimeout(t, serveTimeout(router, "/api/timeout/stopped"))
}

func TestTimeoutHandsAPanicToRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery(), middleware.Timeout(timeoutUnderTest))
	router.GET("/api/timeout/panic", func(*gin.Context) { panic("handler failed") })

	w := serveTimeout(router, "/api/timeout/panic")
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

// newTimeoutRouter mounts Timeout behind a middleware that sets a header of
// its own, which a dropped late response must not take down with it.
func newTimeoutRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Header("X-Upstream", "kept")
		c.Next()
	}, middleware.Timeout(timeoutUnderTest))
	return router
}

func serveTimeout(router *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// requireGatewayTimeout asserts the 504 envelope Timeout answers with.
func requireGatewayTimeout(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()

	require.Equal(t, http.StatusGatewayTimeout, w.Code)
	var envelope struct {
		Msg string `json:"msg"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope), w.Body.String())
	require.Equal(t, "request timeout", envelope.Msg)
}
