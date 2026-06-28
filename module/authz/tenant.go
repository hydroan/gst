package authz

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/middleware"
)

// HeaderTenantResolver resolves the request tenant from an HTTP header.
// Use it for tests, demos, or headers injected by a trusted gateway.
func HeaderTenantResolver(header string) middleware.TenantResolver {
	header = strings.TrimSpace(header)
	return func(c *gin.Context) (string, error) {
		if c == nil || header == "" {
			return "", nil
		}
		return c.GetHeader(header), nil
	}
}
