package middleware

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	. "github.com/hydroan/gst/response"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// Authz authorizes requests using RBAC.
// It derives subject from context or headers, falling back to system user.
// Authz must be called before config.Init so config.Init can read
// AUTH_RBAC_ENABLE from the environment and enable RBAC initialization.
func Authz() gin.HandlerFunc {
	os.Setenv(config.AUTH_RBAC_ENABLE, "true")

	return func(c *gin.Context) {
		var allow bool
		var err error
		sub := c.GetString(consts.CTX_USERNAME)
		obj := c.Request.URL.Path
		act := c.Request.Method

		// The "root" and "admin" is super admin user, can access all resources
		// If subject is not "root" or "admin", use user id as subject
		if sub != consts.AUTHZ_USER_ROOT && sub != consts.AUTHZ_USER_ADMIN {
			sub = c.GetString(consts.CTX_USER_ID)
		}
		if len(sub) == 0 {
			if h := c.GetHeader("X-Username"); len(h) > 0 {
				sub = h
			}
		}
		if len(sub) == 0 {
			if h := c.GetHeader("X-User-Id"); len(h) > 0 {
				sub = h
			}
		}
		if len(sub) == 0 {
			sub = consts.AUTHZ_USER_BLOCKED
		}
		// When RBAC is disabled, Enforcer is nil; skip enforcement and allow the request.
		if rbac.Enforcer == nil {
			c.Next()
			return
		}
		if allow, err = rbac.Enforcer.Enforce(sub, obj, act); err != nil {
			zap.S().Error(err)
			JSON(c, CodeFailure)
			c.Abort()
			return
		}
		if allow {
			c.Next()
			logger.Authz.Infoz(
				"",
				zap.String("sub", sub),
				zap.String("obj", obj),
				zap.String("act", act),
				zap.String("eft", string(consts.EffectAllow)),
				zap.String("username", c.GetString(consts.CTX_USERNAME)),
				zap.String("trace_id", c.GetString(consts.TRACE_ID)),
			)
		} else {
			JSON(c, CodeForbidden)
			c.Abort()
			logger.Authz.Infoz(
				"",
				zap.String("sub", sub),
				zap.String("obj", obj),
				zap.String("act", act),
				zap.String("eft", string(consts.EffectDeny)),
			)
			return

		}
		c.Next()
	}
}
