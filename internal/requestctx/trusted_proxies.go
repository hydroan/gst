package requestctx

import (
	"net/netip"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
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

// ApplyTrustedProxies declares to engine which peers may speak for the
// client, the stance ClientIP takes for the gRPC listener.
//
// Gin trusts every peer out of the box, which makes the client address the
// value the caller wrote into X-Forwarded-For — an address the request picks
// for itself. That address reaches rate limiter keys, audit records, session
// records and the access log, so the stance here is to trust nothing by
// default: an unconfigured deployment reads the address off the connection,
// which no header can rewrite. A deployment behind a load balancer or
// ingress names that network in server.trusted_proxies, the configured
// entries, and the forwarding headers of those peers alone are then
// believed. An entry that cannot be read fails, for startup to stop instead
// of quietly leaving the process on a stance nobody chose.
func ApplyTrustedProxies(engine *gin.Engine, configured []string) error {
	proxies, err := TrustedProxies(configured)
	if err != nil {
		return err
	}
	networks := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		networks = append(networks, proxy.String())
	}
	return errors.Wrap(engine.SetTrustedProxies(networks), "invalid server.trusted_proxies")
}

// forwardingHeaders are the headers a proxy names the client in, read in
// this order: gin's own default list, which the HTTP listener reads.
var forwardingHeaders = []string{"X-Forwarded-For", "X-Real-IP"}

// ClientIP resolves the client address of a call that arrived from peer
// under the stance proxies, the networks whose peers may speak for the
// client, reading the forwarding headers of the call through header by
// name. It names the client gin names for the HTTP listener under the same
// stance (gin.Context.ClientIP): a peer outside the networks speaks for
// itself; from a peer inside them X-Forwarded-For is read from its right
// end, skipping the proxies, down to the first address that is not one,
// or the leftmost when every hop is one; a header holding something that
// is no address is passed over for the next, X-Real-IP, and when no header
// yields an address the peer is the client.
//
// The resolution is the framework's own rather than the realip
// interceptor of go-grpc-middleware, which names a different client in
// three cases of that rule: it answers the peer when X-Forwarded-For is
// absent or malformed, without reading X-Real-IP, and when every hop of
// X-Forwarded-For is a proxy.
func ClientIP(peer netip.Addr, proxies []netip.Prefix, header func(name string) string) netip.Addr {
	if !inNetworks(peer, proxies) {
		return peer
	}
	for _, name := range forwardingHeaders {
		if ip, ok := forwardedIP(header(name), proxies); ok {
			return ip
		}
	}
	return peer
}

// forwardedIP reads the client out of one forwarding header value the way
// ClientIP describes; false when the value names no client.
func forwardedIP(value string, proxies []netip.Prefix) (netip.Addr, bool) {
	if value == "" {
		return netip.Addr{}, false
	}
	for i, hop := range slices.Backward(strings.Split(value, ",")) {
		ip, err := netip.ParseAddr(strings.TrimSpace(hop))
		if err != nil {
			return netip.Addr{}, false
		}
		if i == 0 || !inNetworks(ip, proxies) {
			return ip, true
		}
	}
	return netip.Addr{}, false
}

// inNetworks reports whether one of networks includes ip.
func inNetworks(ip netip.Addr, networks []netip.Prefix) bool {
	return slices.ContainsFunc(networks, func(network netip.Prefix) bool { return network.Contains(ip) })
}
