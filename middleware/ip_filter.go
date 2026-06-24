package middleware

import (
	"net"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	. "github.com/hydroan/gst/internal/response"
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

	// TrustedProxies contains IP addresses of trusted proxy servers
	// Used to correctly extract the real client IP from X-Forwarded-For header
	TrustedProxies []string
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
		// Get client IP
		clientIP := getClientIP(c, config.TrustedProxies)
		ip := net.ParseIP(clientIP)
		if ip == nil {
			zap.S().Warnw("failed to parse client IP", "ip", clientIP)
			JSON(c, CodeForbidden.WithMsg("invalid client IP"))
			c.Abort()
			return
		}

		// Check blacklist first (blacklist takes precedence)
		if slices.ContainsFunc(blacklistIPs, func(blockedIP net.IP) bool {
			return ip.Equal(blockedIP)
		}) {
			zap.S().Warnw("request blocked by blacklist", "ip", clientIP)
			JSON(c, CodeForbidden.WithMsg("access denied"))
			c.Abort()
			return
		}
		for _, blockedNet := range blacklistNets {
			if blockedNet.Contains(ip) {
				zap.S().Warnw("request blocked by blacklist", "ip", clientIP, "cidr", blockedNet.String())
				JSON(c, CodeForbidden.WithMsg("access denied"))
				c.Abort()
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
				JSON(c, CodeForbidden.WithMsg("access denied"))
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// getClientIP extracts the real client IP from the request, considering proxy headers.
func getClientIP(c *gin.Context, trustedProxies []string) string {
	// First, try to get IP from X-Forwarded-For header if proxy is trusted
	if len(trustedProxies) > 0 {
		forwardedFor := c.GetHeader("X-Forwarded-For")
		if forwardedFor != "" {
			// X-Forwarded-For can contain multiple IPs, take the last one (closest to server)
			ips := strings.Split(forwardedFor, ",")
			if len(ips) > 0 {
				// Get the last IP in the chain (closest to the server)
				clientIP := strings.TrimSpace(ips[len(ips)-1])
				// Verify the proxy is trusted
				proxyIP := c.ClientIP()
				if isTrustedProxy(proxyIP, trustedProxies) {
					return util.IPv6ToIPv4(clientIP)
				}
			}
		}

		// Try X-Real-IP header
		realIP := c.GetHeader("X-Real-IP")
		if realIP != "" {
			proxyIP := c.ClientIP()
			if isTrustedProxy(proxyIP, trustedProxies) {
				return util.IPv6ToIPv4(realIP)
			}
		}
	}

	// Fall back to gin's ClientIP() method
	return util.IPv6ToIPv4(c.ClientIP())
}

// isTrustedProxy checks if the given IP is in the trusted proxies list.
func isTrustedProxy(ip string, trustedProxies []string) bool {
	proxyIP := net.ParseIP(ip)
	if proxyIP == nil {
		return false
	}

	for _, trusted := range trustedProxies {
		if strings.Contains(trusted, "/") {
			_, ipNet, err := net.ParseCIDR(trusted)
			if err != nil {
				continue
			}
			if ipNet.Contains(proxyIP) {
				return true
			}
		} else {
			trustedIP := net.ParseIP(trusted)
			if trustedIP != nil && proxyIP.Equal(trustedIP) {
				return true
			}
		}
	}

	return false
}
