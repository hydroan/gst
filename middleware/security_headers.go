package middleware

import (
	"github.com/gin-gonic/gin"
)

// SecurityHeadersConfig holds configuration for security headers middleware
type SecurityHeadersConfig struct {
	// XFrameOptions controls the X-Frame-Options header
	// Options: "DENY", "SAMEORIGIN", or empty string to disable
	XFrameOptions string

	// XContentTypeOptions controls the X-Content-Type-Options header
	// Set to "nosniff" to enable, or empty string to disable
	XContentTypeOptions string

	// XXSSProtection controls the X-XSS-Protection header
	// Set to "1; mode=block" to enable, or empty string to disable
	XXSSProtection string

	// StrictTransportSecurity controls the Strict-Transport-Security header
	// Set to a value like "max-age=31536000; includeSubDomains" to enable, or empty string to disable
	StrictTransportSecurity string

	// ContentSecurityPolicy controls the Content-Security-Policy header
	// Set to a CSP policy string to enable, or empty string to disable
	ContentSecurityPolicy string

	// ReferrerPolicy controls the Referrer-Policy header
	// Options: "no-referrer", "no-referrer-when-downgrade", "origin", etc., or empty string to disable
	ReferrerPolicy string

	// PermissionsPolicy controls the Permissions-Policy header (formerly Feature-Policy)
	// Set to a permissions policy string to enable, or empty string to disable
	PermissionsPolicy string
}

// SecurityHeaders returns a middleware that sets security-related HTTP headers.
// This helps protect against various web vulnerabilities.
//
// Parameters:
//   - config: Configuration for security headers. If nil, default secure headers will be used.
//
// Returns:
//   - A gin.HandlerFunc that sets security headers
//
// Example:
//
//	// Use default secure headers
//	router.Use(middleware.SecurityHeaders(nil))
//
//	// Use custom configuration
//	router.Use(middleware.SecurityHeaders(&middleware.SecurityHeadersConfig{
//		XFrameOptions:            "DENY",
//		XContentTypeOptions:      "nosniff",
//		XXSSProtection:           "1; mode=block",
//		StrictTransportSecurity:  "max-age=31536000; includeSubDomains",
//		ContentSecurityPolicy:    "default-src 'self'",
//		ReferrerPolicy:           "strict-origin-when-cross-origin",
//	}))
func SecurityHeaders(config *SecurityHeadersConfig) gin.HandlerFunc {
	// Use default config if none provided
	if config == nil {
		config = &SecurityHeadersConfig{
			XFrameOptions:           "SAMEORIGIN",
			XContentTypeOptions:     "nosniff",
			XXSSProtection:          "1; mode=block",
			StrictTransportSecurity: "max-age=31536000; includeSubDomains",
			ReferrerPolicy:          "strict-origin-when-cross-origin",
		}
	}

	return func(c *gin.Context) {
		if config.XFrameOptions != "" {
			c.Header("X-Frame-Options", config.XFrameOptions)
		}
		if config.XContentTypeOptions != "" {
			c.Header("X-Content-Type-Options", config.XContentTypeOptions)
		}
		if config.XXSSProtection != "" {
			c.Header("X-XSS-Protection", config.XXSSProtection)
		}
		if config.StrictTransportSecurity != "" {
			c.Header("Strict-Transport-Security", config.StrictTransportSecurity)
		}
		if config.ContentSecurityPolicy != "" {
			c.Header("Content-Security-Policy", config.ContentSecurityPolicy)
		}
		if config.ReferrerPolicy != "" {
			c.Header("Referrer-Policy", config.ReferrerPolicy)
		}
		if config.PermissionsPolicy != "" {
			c.Header("Permissions-Policy", config.PermissionsPolicy)
		}

		c.Next()
	}
}
