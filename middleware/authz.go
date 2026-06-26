package middleware

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// Authz authorizes requests using RBAC.
// It derives subject from trusted request context and blocks anonymous requests.
// Authz must be called before config.Init so config.Init can read
// AUTH_RBAC_ENABLE from the environment and enable RBAC initialization.
func Authz() gin.HandlerFunc {
	os.Setenv(config.AUTH_RBAC_ENABLE, "true")

	return func(c *gin.Context) {
		var allow bool
		var err error
		sub := c.GetString(consts.CTX_USER_ID)
		if len(sub) == 0 {
			sub = consts.AUTHZ_USER_BLOCKED
		}
		obj := c.Request.URL.Path
		act := c.Request.Method

		if rbac.Enforcer == nil {
			zap.S().Error("Authz middleware invoked but RBAC enforcer is nil")
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":          -1,
				"msg":           "authorization failed",
				"data":          nil,
				consts.TRACE_ID: c.GetString(consts.TRACE_ID),
			})
			return
		}
		if allow, err = rbac.Enforcer.Enforce(sub, obj, act); err != nil {
			zap.S().Error(err)
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":          -1,
				"msg":           "authorization failed",
				"data":          nil,
				consts.TRACE_ID: c.GetString(consts.TRACE_ID),
			})
			return
		}
		if allow {
			c.Next()
			if logger.Authz != nil {
				logger.Authz.Infoz(
					"",
					zap.String("sub", sub),
					zap.String("obj", obj),
					zap.String("act", act),
					zap.String("eft", string(consts.EffectAllow)),
					zap.String("username", c.GetString(consts.CTX_USERNAME)),
					zap.String("trace_id", c.GetString(consts.TRACE_ID)),
				)
			}
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":          -1,
			"msg":           "permission denied",
			"data":          nil,
			consts.TRACE_ID: c.GetString(consts.TRACE_ID),
		})
		if logger.Authz != nil {
			logger.Authz.Infoz(
				"",
				zap.String("sub", sub),
				zap.String("obj", obj),
				zap.String("act", act),
				zap.String("eft", string(consts.EffectDeny)),
			)
		}
	}
}
