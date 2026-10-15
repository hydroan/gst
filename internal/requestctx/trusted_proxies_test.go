package requestctx_test

import (
	"net/netip"
	"testing"

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
