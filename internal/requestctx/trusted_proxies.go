package requestctx

import (
	"net/netip"
	"strings"

	"github.com/cockroachdb/errors"
)

// TrustedProxies turns the entries of server.trusted_proxies into the
// networks whose peers may speak for the client, for both listeners to
// believe the same forwarding headers from the same peers: a CIDR block
// stays as it is, and a lone address names the network of itself alone, so
// an entry reads the same way on either listener. Surrounding spaces are
// trimmed and blank entries dropped, so that a list written as
// "10.0.0.0/8, 192.168.0.0/16" and an environment variable set to nothing
// both parse. An entry that is neither fails, for startup to stop on a
// stance nobody chose.
func TrustedProxies(configured []string) ([]netip.Prefix, error) {
	proxies := make([]netip.Prefix, 0, len(configured))
	for _, entry := range configured {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			proxies = append(proxies, prefix)
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, errors.Newf("server.trusted_proxies entry %q is neither a CIDR block nor an address", entry)
		}
		proxies = append(proxies, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return proxies, nil
}
