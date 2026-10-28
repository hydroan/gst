package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/middleware"
	"github.com/stretchr/testify/require"
)

// TestSecurityHeadersSetsTheDefaultsWithoutAConfig pins the headers
// SecurityHeaders(nil) sets on every response, and that it leaves the
// content security and permissions policies to the project.
func TestSecurityHeadersSetsTheDefaultsWithoutAConfig(t *testing.T) {
	header := serveSecurityHeaders(nil)

	require.Equal(t, "SAMEORIGIN", header.Get("X-Frame-Options"))
	require.Equal(t, "nosniff", header.Get("X-Content-Type-Options"))
	require.Equal(t, "1; mode=block", header.Get("X-XSS-Protection"))
	require.Equal(t, "max-age=31536000; includeSubDomains", header.Get("Strict-Transport-Security"))
	require.Equal(t, "strict-origin-when-cross-origin", header.Get("Referrer-Policy"))
	require.Empty(t, header.Values("Content-Security-Policy"))
	require.Empty(t, header.Values("Permissions-Policy"))
}

// TestSecurityHeadersSetsWhatTheConfigNames pins a config of the project's
// own: each header it gives a value is set to that value, and each it leaves
// empty is not set at all, the defaults included.
func TestSecurityHeadersSetsWhatTheConfigNames(t *testing.T) {
	header := serveSecurityHeaders(&middleware.SecurityHeadersConfig{
		XFrameOptions:         "DENY",
		ContentSecurityPolicy: "default-src 'self'",
		PermissionsPolicy:     "camera=()",
	})

	require.Equal(t, "DENY", header.Get("X-Frame-Options"))
	require.Equal(t, "default-src 'self'", header.Get("Content-Security-Policy"))
	require.Equal(t, "camera=()", header.Get("Permissions-Policy"))
	for _, name := range []string{"X-Content-Type-Options", "X-XSS-Protection", "Strict-Transport-Security", "Referrer-Policy"} {
		require.Empty(t, header.Values(name), name)
	}
}

// serveSecurityHeaders answers a request through SecurityHeaders(config) and
// returns the response headers.
func serveSecurityHeaders(config *middleware.SecurityHeadersConfig) http.Header {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.SecurityHeaders(config))
	router.GET("/api/page", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/page", nil))
	return w.Header()
}
