package router_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

const clientIPRoute = "/client-ip"

func registerClientIPRoute() {
	router.Pub().GET(clientIPRoute, func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})
}

// TestClientIPIgnoresForgedForwardingHeaders is the end-to-end half of the
// stance router.Init installs: over a real connection, a caller sending its
// own X-Forwarded-For and X-Real-IP is still recorded as the address it
// connected from. Everything keyed on the client address — rate limits, audit
// records, session records, the access log — rests on this.
func TestClientIPIgnoresForgedForwardingHeaders(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+consts.APIPathPrefix+clientIPRoute, nil)
	require.NoError(t, err)
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("X-Real-IP", "198.51.100.4")

	rsp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer rsp.Body.Close()

	body, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", string(body))
}
