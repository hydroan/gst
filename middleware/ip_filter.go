package middleware

import (
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/response"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// IPFilterConfig holds configuration for IP filtering middleware
type IPFilterConfig struct {
	// Whitelist contains allowed IP addresses or CIDR ranges
	// If non-empty, only IPs in this list will be allowed
	Whitelist []string

	// Blacklist contains blocked IP addresses or CIDR ranges
	// IPs in this list will always be blocked
	Blacklist []string
}

// IPWhitelist returns a middleware that only allows requests from IP addresses in the whitelist.
//
// Parameters:
//   - whitelist: List of allowed IP addresses or CIDR ranges (e.g., "192.168.1.1", "10.0.0.0/8")
//
// Returns:
//   - A gin.HandlerFunc that enforces IP whitelist
//
// Example:
//
//	// Allow only specific IPs
//	router.Use(middleware.IPWhitelist([]string{"192.168.1.1", "10.0.0.0/8"}))
//
//	// Allow only localhost
//	router.Use(middleware.IPWhitelist([]string{"127.0.0.1", "::1"}))
func IPWhitelist(whitelist []string) gin.HandlerFunc {
	config := &IPFilterConfig{
		Whitelist: whitelist,
	}
	return IPFilter(config)
}

// IPBlacklist returns a middleware that blocks requests from IP addresses in the blacklist.
//
// Parameters:
//   - blacklist: List of blocked IP addresses or CIDR ranges (e.g., "192.168.1.100", "10.0.0.0/8")
//
// Returns:
//   - A gin.HandlerFunc that enforces IP blacklist
//
// Example:
//
//	// Block specific IPs
//	router.Use(middleware.IPBlacklist([]string{"192.168.1.100", "10.0.0.0/8"}))
//
//	// Block known malicious IPs
//	router.Use(middleware.IPBlacklist([]string{"1.2.3.4", "5.6.7.8"}))
func IPBlacklist(blacklist []string) gin.HandlerFunc {
	config := &IPFilterConfig{
		Blacklist: blacklist,
	}
	return IPFilter(config)
}

// IPFilter returns a middleware that filters requests based on IP whitelist and blacklist.
// Blacklist takes precedence over whitelist.
//
// The address filtered on is the one the engine reports: the peer of the
// connection, or the address a trusted proxy forwarded. Which proxies count is
// server.trusted_proxies, applied to the engine at startup — behind a proxy
// that is not named there, every request filters as the proxy's own address.
//
// Parameters:
//   - config: Configuration for IP filtering
//
// Returns:
//   - A gin.HandlerFunc that enforces IP filtering rules
//
// Example:
//
//	// Use both whitelist and blacklist
//	router.Use(middleware.IPFilter(&middleware.IPFilterConfig{
//		Whitelist: []string{"192.168.0.0/16"},
//		Blacklist: []string{"192.168.1.100"},
//	}))
func IPFilter(config *IPFilterConfig) gin.HandlerFunc {
	if config == nil {
		config = &IPFilterConfig{}
	}

	// Parse CIDR ranges and IPs into net.IPNet and net.IP for efficient matching
	whitelistNets := make([]*net.IPNet, 0)
	whitelistIPs := make([]net.IP, 0)
	for _, item := range config.Whitelist {
		if strings.Contains(item, "/") {
			_, ipNet, err := net.ParseCIDR(item)
			if err != nil {
				zap.S().Warnw("invalid CIDR in whitelist", "cidr", item, "error", err)
				continue
			}
			whitelistNets = append(whitelistNets, ipNet)
		} else {
			ip := net.ParseIP(item)
			if ip == nil {
				zap.S().Warnw("invalid IP in whitelist", "ip", item)
				continue
			}
			whitelistIPs = append(whitelistIPs, ip)
		}
	}

	blacklistNets := make([]*net.IPNet, 0)
	blacklistIPs := make([]net.IP, 0)
	for _, item := range config.Blacklist {
		if strings.Contains(item, "/") {
			_, ipNet, err := net.ParseCIDR(item)
			if err != nil {
				zap.S().Warnw("invalid CIDR in blacklist", "cidr", item, "error", err)
				continue
			}
			blacklistNets = append(blacklistNets, ipNet)
		} else {
			ip := net.ParseIP(item)
			if ip == nil {
				zap.S().Warnw("invalid IP in blacklist", "ip", item)
				continue
			}
			blacklistIPs = append(blacklistIPs, ip)
		}
	}

	return func(c *gin.Context) {
		// The address is the one the engine settled on, which is the peer of
		// the connection unless a trusted proxy forwarded another. Whether a
		// forwarding header counts is decided once for the whole server by
		// server.trusted_proxies, so this filter never has to judge it.
		clientIP := util.IPv6ToIPv4(c.ClientIP())
		ip := net.ParseIP(clientIP)
		if ip == nil {
			zap.S().Warnw("failed to parse client IP", "ip", clientIP)
			response.Abort(c, http.StatusForbidden, "invalid client IP")
			return
		}

		// Check blacklist first (blacklist takes precedence)
		if slices.ContainsFunc(blacklistIPs, func(blockedIP net.IP) bool {
			return ip.Equal(blockedIP)
		}) {
			zap.S().Warnw("request blocked by blacklist", "ip", clientIP)
			response.Abort(c, http.StatusForbidden, "access denied")
			return
		}
		for _, blockedNet := range blacklistNets {
			if blockedNet.Contains(ip) {
				zap.S().Warnw("request blocked by blacklist", "ip", clientIP, "cidr", blockedNet.String())
				response.Abort(c, http.StatusForbidden, "access denied")
				return
			}
		}

		// Check whitelist if configured
		if len(config.Whitelist) > 0 {
			// Check exact IP match
			allowed := slices.ContainsFunc(whitelistIPs, func(allowedIP net.IP) bool {
				return ip.Equal(allowedIP)
			})

			// Check CIDR match
			if !allowed {
				allowed = slices.ContainsFunc(whitelistNets, func(allowedNet *net.IPNet) bool {
					return allowedNet.Contains(ip)
				})
			}

			if !allowed {
				zap.S().Warnw("request blocked by whitelist", "ip", clientIP)
				response.Abort(c, http.StatusForbidden, "access denied")
				return
			}
		}

		c.Next()
	}
}
