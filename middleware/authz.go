package middleware

import (
	"net/http"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/response"
	"github.com/hydroan/gst/service"
)

// Authz authorizes requests using RBAC, through rbac.Enforce, the one
// decision path the gRPC interceptor of the module takes as well: the
// subject is read off the gin context, the object and action are the
// request path and method, and a refusal is answered in the API envelope
// with the status and message the service error carries. Authz must be
// called before config.Init so config.Init can read AUTH_RBAC_ENABLED from
// the environment and enable RBAC initialization.
//
// Authz must run after an authentication middleware that populates
// consts.CTX_USER_ID. When using built-in IAM sessions, register IAMSession
// before Authz; otherwise a valid session cookie is rejected as "permission
// denied" because Authz cannot find the authenticated subject yet.
//
// The request tenant has exactly one source: consts.CTX_TENANT_ID, read as it
// stands when this middleware runs, defaulted to tenant.Default when empty.
// Whatever trusted middleware wrote it last decides. IAMSession writes the
// session's tenant; a deployment whose tenant arrives another way — a header a
// trusted gateway injects, a subdomain — installs its own middleware after
// IAMSession and before Authz and overwrites it. The value must only ever be
// written from something the deployment vouches for, never from client input
// passed through as it stands; rbac.Enforce says why.
func Authz() gin.HandlerFunc {
	os.Setenv(config.AUTH_RBAC_ENABLED, "true")

	return func(c *gin.Context) {
		subject := rbac.Subject{
			UserID:   c.GetString(consts.CTX_USER_ID),
			Username: c.GetString(consts.CTX_USERNAME),
			TenantID: c.GetString(consts.CTX_TENANT_ID),
		}
		ctx, tenantID, err := rbac.Enforce(c.Request.Context(), subject, c.Request.URL.Path, c.Request.Method)
		if err != nil {
			status, msg := http.StatusInternalServerError, "internal server error"
			var serviceErr *service.Error
			if errors.As(err, &serviceErr) {
				status, msg = serviceErr.Status(), serviceErr.Msg()
			}
			response.Abort(c, status, msg)
			return
		}
		c.Set(consts.CTX_TENANT_ID, tenantID)
		c.Request = c.Request.WithContext(ctx)
	}
}
