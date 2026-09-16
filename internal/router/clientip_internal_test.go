package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// withTrustedProxies installs a trusted-proxy configuration for one test.
func withTrustedProxies(t *testing.T, proxies ...string) {
	t.Helper()

	original := config.App.Server.TrustedProxies
	t.Cleanup(func() { config.App.Server.TrustedProxies = original })
	config.App.Server.TrustedProxies = proxies
}

// clientIPFrom builds an engine carrying the configured stance, sends it one
// request arriving from peer and claiming to forward forwarded, and reports
// the client address the engine settled on.
func clientIPFrom(t *testing.T, peer, forwarded string) string {
	t.Helper()

	engine := gin.New()
	require.NoError(t, applyTrustedProxies(engine))
	engine.GET("/probe", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.RemoteAddr = peer
	req.Header.Set("X-Forwarded-For", forwarded)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder.Body.String()
}

// TestClientIPWithoutTrustedProxiesComesFromTheConnection pins the default
// stance: with no proxy configured, the address a request claims to forward
// is ignored in favor of the peer it actually arrived from.
func TestClientIPWithoutTrustedProxiesComesFromTheConnection(t *testing.T) {
	withTrustedProxies(t)

	require.Equal(t, "10.1.2.3", clientIPFrom(t, "10.1.2.3:41000", "203.0.113.9"))
}

// TestClientIPFromTrustedProxyUsesForwardedAddress covers the deployment the
// configuration exists for: behind a balancer whose network is named, the
// address that balancer forwards is the client.
func TestClientIPFromTrustedProxyUsesForwardedAddress(t *testing.T) {
	withTrustedProxies(t, "10.1.2.0/24")

	require.Equal(t, "203.0.113.9", clientIPFrom(t, "10.1.2.3:41000", "203.0.113.9"))
}

// TestClientIPFromUntrustedPeerIgnoresForwardedAddress asserts the trust is
// per peer rather than global: a request reaching the server from outside the
// configured network speaks only for itself, whatever it forwards.
func TestClientIPFromUntrustedPeerIgnoresForwardedAddress(t *testing.T) {
	withTrustedProxies(t, "10.1.2.0/24")

	require.Equal(t, "192.0.2.7", clientIPFrom(t, "192.0.2.7:41000", "203.0.113.9"))
}

// TestTrustedProxiesNormalizesConfiguredEntries covers the two shapes a hand
// written list arrives in: entries spaced out after the separator, and an
// environment variable set to nothing at all.
func TestTrustedProxiesNormalizesConfiguredEntries(t *testing.T) {
	withTrustedProxies(t, " 10.1.2.0/24 ", "", "   ")

	require.Equal(t, []string{"10.1.2.0/24"}, trustedProxies())
}

// TestApplyTrustedProxiesRejectsUnparseableEntry asserts startup stops on a
// configuration gin cannot read, rather than running on a stance nobody chose.
func TestApplyTrustedProxiesRejectsUnparseableEntry(t *testing.T) {
	withTrustedProxies(t, "not-a-cidr")

	require.Error(t, applyTrustedProxies(gin.New()))
}
