package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hydroan/gst/internal/consts"
	"github.com/stretchr/testify/require"
)

// TestInitBelievesTheProxiesTheConfigurationNames pins the wiring Init does:
// the networks server.trusted_proxies names, 10.0.0.0/8 here (see TestMain),
// are the peers whose forwarded address the engine believes, and a peer
// outside them speaks for itself. The requests reach the client-ip route of
// clientip_test.go through the engine Init built, from the peers named here.
func TestInitBelievesTheProxiesTheConfigurationNames(t *testing.T) {
	clientIP := func(peer string) string {
		req := httptest.NewRequest(http.MethodGet, consts.APIPath("/client-ip"), nil)
		req.RemoteAddr = peer + ":41000"
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		recorder := httptest.NewRecorder()
		root.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		return recorder.Body.String()
	}

	require.Equal(t, "203.0.113.9", clientIP("10.1.2.3"), "a peer of the configured network speaks for the client")
	require.Equal(t, "192.0.2.7", clientIP("192.0.2.7"), "any other peer speaks for itself")
}
