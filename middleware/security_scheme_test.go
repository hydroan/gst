package middleware_test

import (
	"testing"

	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/middleware"
	"github.com/stretchr/testify/require"
)

// TestAuthMiddlewareDeclareTheirSecuritySchemes pins that building the IAM
// session middleware declares the session cookie to the OpenAPI document and
// building the JWT middleware declares the bearer token, so the document of
// a project names the schemes it mounts and no other.
func TestAuthMiddlewareDeclareTheirSecuritySchemes(t *testing.T) {
	middleware.IAMSession()
	middleware.JwtAuth()

	schemes := apidoc.SecuritySchemes()
	require.Equal(t, apidoc.SecurityScheme{Type: "apiKey", In: "cookie", Name: "session_id", Description: "IAM session cookie issued by POST /api/login"}, schemes["cookieAuth"])
	require.Equal(t, apidoc.SecurityScheme{Type: "http", Scheme: "bearer"}, schemes["bearerAuth"])
}
