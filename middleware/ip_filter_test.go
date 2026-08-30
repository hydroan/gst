package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// filterStatus runs one request through handler and reports the status the
// filter answered with. peer is the address the request arrives from, and
// forwarded, when set, the address it claims to carry; trustedProxies is the
// engine-wide stance that decides whether the claim counts.
func filterStatus(t *testing.T, handler gin.HandlerFunc, peer, forwarded string, trustedProxies ...string) int {
	t.Helper()

	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(trustedProxies))
	engine.Use(handler)
	engine.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.RemoteAddr = peer
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
	}

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder.Code
}

func TestIPWhitelistAdmitsListedAddressesOnly(t *testing.T) {
	handler := IPWhitelist([]string{"10.1.2.3", "192.168.0.0/16"})

	require.Equal(t, http.StatusOK, filterStatus(t, handler, "10.1.2.3:41000", ""), "the listed address")
	require.Equal(t, http.StatusOK, filterStatus(t, handler, "192.168.4.5:41000", ""), "an address inside the listed range")
	require.Equal(t, http.StatusForbidden, filterStatus(t, handler, "203.0.113.9:41000", ""), "an address on neither list entry")
}

func TestIPBlacklistRejectsListedAddressesOnly(t *testing.T) {
	handler := IPBlacklist([]string{"10.1.2.3", "192.168.0.0/16"})

	require.Equal(t, http.StatusForbidden, filterStatus(t, handler, "10.1.2.3:41000", ""), "the listed address")
	require.Equal(t, http.StatusForbidden, filterStatus(t, handler, "192.168.4.5:41000", ""), "an address inside the listed range")
	require.Equal(t, http.StatusOK, filterStatus(t, handler, "203.0.113.9:41000", ""), "an address on neither list entry")
}

// TestIPFilterBlacklistOutranksWhitelist pins the documented precedence: an
// address inside an allowed range is still refused when it is named as blocked.
func TestIPFilterBlacklistOutranksWhitelist(t *testing.T) {
	handler := IPFilter(&IPFilterConfig{
		Whitelist: []string{"192.168.0.0/16"},
		Blacklist: []string{"192.168.1.100"},
	})

	require.Equal(t, http.StatusOK, filterStatus(t, handler, "192.168.1.99:41000", ""))
	require.Equal(t, http.StatusForbidden, filterStatus(t, handler, "192.168.1.100:41000", ""))
}

// TestIPFilterFiltersForwardedAddressBehindTrustedProxy is why this filter no
// longer reads forwarding headers itself: behind a proxy the engine trusts,
// the address on the lists is the client's, not the proxy's — so a blocked
// client stays blocked even though every request arrives from the same peer.
func TestIPFilterFiltersForwardedAddressBehindTrustedProxy(t *testing.T) {
	handler := IPBlacklist([]string{"203.0.113.9"})
	const proxy = "10.1.2.3:41000"

	require.Equal(t, http.StatusForbidden,
		filterStatus(t, handler, proxy, "203.0.113.9", "10.1.2.0/24"), "the blocked client behind the proxy")
	require.Equal(t, http.StatusOK,
		filterStatus(t, handler, proxy, "198.51.100.4", "10.1.2.0/24"), "another client behind the same proxy")
}

// TestIPFilterIgnoresForwardedAddressFromUntrustedPeer is the other half: a
// caller cannot lift its own ban, or borrow someone's allowance, by naming an
// address in a header the server has no reason to believe.
func TestIPFilterIgnoresForwardedAddressFromUntrustedPeer(t *testing.T) {
	require.Equal(t, http.StatusForbidden,
		filterStatus(t, IPBlacklist([]string{"203.0.113.9"}), "203.0.113.9:41000", "198.51.100.4"),
		"a blocked caller forging an allowed address")
	require.Equal(t, http.StatusForbidden,
		filterStatus(t, IPWhitelist([]string{"10.1.2.3"}), "203.0.113.9:41000", "10.1.2.3"),
		"an unlisted caller forging a listed address")
}
