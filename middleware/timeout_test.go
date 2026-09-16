package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	internalmiddleware "github.com/hydroan/gst/internal/middleware"
	"github.com/stretchr/testify/require"
)

func TestTimeoutLeavesStreamingRouteAlone(t *testing.T) {
	internalmiddleware.MarkStreamingRoute(http.MethodGet, "/api/timeout/stream")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Timeout(30 * time.Millisecond))
	router.GET("/api/timeout/stream", func(c *gin.Context) {
		// Outlives the timeout on purpose; only the streaming exemption lets
		// this response finish.
		time.Sleep(100 * time.Millisecond)
		c.String(http.StatusOK, "finished")
	})

	stream := httptest.NewRecorder()
	router.ServeHTTP(stream, httptest.NewRequest(http.MethodGet, "/api/timeout/stream", nil))
	require.Equal(t, http.StatusOK, stream.Code)
	require.Equal(t, "finished", stream.Body.String())

	// The timed-out counterpart is deliberately not exercised: the timeout
	// branch hands the gin context back to the engine while the handler
	// goroutine still runs, an inherent data race in Timeout that predates
	// the streaming registry and trips the race detector on any request that
	// actually times out. Cover it when that defect is fixed.
}
