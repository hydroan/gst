package requestctx_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/stretchr/testify/require"
)

// TestTrustedProxiesParsesNetworksAndAddresses pins what server.trusted_proxies
// turns into for both listeners: a CIDR block as it is, a lone address as the
// network of itself alone, surrounding spaces trimmed and blank entries
// dropped; an entry that is neither fails, naming it.
func TestTrustedProxiesParsesNetworksAndAddresses(t *testing.T) {
	proxies, err := requestctx.TrustedProxies([]string{" 10.1.2.0/24 ", "", "   ", "192.0.2.7", "2001:db8::1"})
	require.NoError(t, err)
	require.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("10.1.2.0/24"),
		netip.MustParsePrefix("192.0.2.7/32"),
		netip.MustParsePrefix("2001:db8::1/128"),
	}, proxies)

	proxies, err = requestctx.TrustedProxies(nil)
	require.NoError(t, err)
	require.Empty(t, proxies)

	_, err = requestctx.TrustedProxies([]string{"10.1.2.0/24", "not-a-cidr"})
	require.ErrorContains(t, err, `"not-a-cidr"`)
}

// TestApplyTrustedProxiesRejectsUnparseableEntry asserts startup stops on a
// configuration that cannot be read, rather than running on a stance nobody
// chose.
func TestApplyTrustedProxiesRejectsUnparseableEntry(t *testing.T) {
	require.Error(t, requestctx.ApplyTrustedProxies(gin.New(), []string{"not-a-cidr"}))
}

// TestClientIPResolvesTheWayGinDoes pins, case by case, that the resolution
// the gRPC listener runs names the client gin names for the HTTP listener
// under the same stance: every case is answered by both, and the answers
// must agree. The stance names 10.0.0.0/8 as the proxies.
func TestClientIPResolvesTheWayGinDoes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configured := []string{"10.0.0.0/8"}
	proxies, err := requestctx.TrustedProxies(configured)
	require.NoError(t, err)
	engine := gin.New()
	require.NoError(t, requestctx.ApplyTrustedProxies(engine, configured))
	engine.GET("/probe", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	for _, tc := range []struct {
		name, peer string
		headers    map[string]string
		want       string
	}{
		{"a peer outside the networks speaks for itself", "192.0.2.7", map[string]string{"X-Forwarded-For": "203.0.113.9"}, "192.0.2.7"},
		{"a proxy forwards the client", "10.1.2.3", map[string]string{"X-Forwarded-For": "203.0.113.9"}, "203.0.113.9"},
		{"the hops through other proxies are skipped", "10.1.2.3", map[string]string{"X-Forwarded-For": "203.0.113.9, 10.0.0.6"}, "203.0.113.9"},
		{"a chain of proxies alone names its first", "10.1.2.3", map[string]string{"X-Forwarded-For": "10.0.0.5, 10.0.0.6"}, "10.0.0.5"},
		{"X-Real-IP alone is read", "10.1.2.3", map[string]string{"X-Real-IP": "203.0.113.9"}, "203.0.113.9"},
		{"a malformed X-Forwarded-For passes to X-Real-IP", "10.1.2.3", map[string]string{"X-Forwarded-For": "junk", "X-Real-IP": "203.0.113.9"}, "203.0.113.9"},
		{"a malformed header alone leaves the peer", "10.1.2.3", map[string]string{"X-Forwarded-For": "junk"}, "10.1.2.3"},
		{"X-Forwarded-For is read before X-Real-IP", "10.1.2.3", map[string]string{"X-Forwarded-For": "203.0.113.9", "X-Real-IP": "198.51.100.1"}, "203.0.113.9"},
		{"no header leaves the peer", "10.1.2.3", nil, "10.1.2.3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := requestctx.ClientIP(netip.MustParseAddr(tc.peer), proxies, func(name string) string { return tc.headers[name] })
			require.Equal(t, tc.want, got.String())

			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			req.RemoteAddr = tc.peer + ":41000"
			for name, value := range tc.headers {
				req.Header.Set(name, value)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			require.Equal(t, tc.want, recorder.Body.String(), "gin names the same client")
		})
	}

	t.Run("with no network named, every peer speaks for itself", func(t *testing.T) {
		got := requestctx.ClientIP(netip.MustParseAddr("10.1.2.3"), nil, func(string) string { return "203.0.113.9" })
		require.Equal(t, "10.1.2.3", got.String())
	})
}
