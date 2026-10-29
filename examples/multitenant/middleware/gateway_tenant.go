package middleware

import (
	"strings"

	"multitenant/configx"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
)

// gatewayTenant takes the tenant from the header a trusted gateway names it
// in, overriding the one the session carries: a deployment whose tenant
// arrives on the request rather than at login. It runs after the session
// check, which wrote the session's tenant, and before the authorization,
// which reads whatever stands in CTX_TENANT_ID when it runs. With no header
// configured it changes nothing, and the session's tenant decides.
func gatewayTenant(c *gin.Context) {
	header := config.Get[configx.Gateway]().TenantHeader
	if header == "" {
		return
	}
	if tenantID := strings.TrimSpace(c.GetHeader(header)); tenantID != "" {
		c.Set(consts.CTX_TENANT_ID, tenantID)
	}
}
