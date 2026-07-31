package middleware

import (
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// TenantResolver resolves the authorization tenant for the current request.
type TenantResolver func(*gin.Context) (string, error)

// AuthzConfig configures RBAC authorization middleware.
type AuthzConfig struct {
	TenantResolver TenantResolver
}

// AuthzOption configures Authz middleware.
type AuthzOption func(*AuthzConfig)

var authzTenantResolver = struct {
	sync.RWMutex
	resolver TenantResolver
}{
	resolver: defaultTenantResolver,
}

// WithTenantResolver sets the request tenant resolver used by Authz.
func WithTenantResolver(resolver TenantResolver) AuthzOption {
	return func(cfg *AuthzConfig) {
		if resolver != nil {
			cfg.TenantResolver = resolver
		}
	}
}

// SetAuthzTenantResolver sets the tenant resolver used by zero-argument Authz.
func SetAuthzTenantResolver(resolver TenantResolver) {
	if resolver == nil {
		resolver = defaultTenantResolver
	}

	authzTenantResolver.Lock()
	defer authzTenantResolver.Unlock()
	authzTenantResolver.resolver = resolver
}

// Authz authorizes requests using RBAC.
// It derives subject from trusted request context and blocks anonymous requests.
// Authz must be called before config.Init so config.Init can read
// AUTH_RBAC_ENABLED from the environment and enable RBAC initialization.
//
// Authz must run after an authentication middleware that populates
// consts.CTX_USER_ID. When using built-in IAM sessions, register IAMSession
// before Authz; otherwise a valid session cookie is rejected as "permission
// denied" because Authz cannot find the authenticated subject yet.
func Authz(options ...AuthzOption) gin.HandlerFunc {
	os.Setenv(config.AUTH_RBAC_ENABLED, "true")

	cfg := AuthzConfig{}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	return func(c *gin.Context) {
		var allow bool
		var err error

		obj := c.Request.URL.Path
		act := c.Request.Method

		sub := strings.TrimSpace(c.GetString(consts.CTX_USER_ID))
		if sub == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":          -1,
				"msg":           "permission denied",
				"data":          nil,
				consts.TRACE_ID: c.GetString(consts.TRACE_ID),
			})
			// Anonymous requests are rejected before the tenant is resolved,
			// so the decision is recorded without one.
			logAuthzDecision(c, "", sub, obj, act, consts.EffectDeny)
			return
		}
		tenant, err := resolveAuthzTenant(c, cfg.TenantResolver)
		if err != nil {
			zap.S().Error(err)
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":          -1,
				"msg":           "authorization failed",
				"data":          nil,
				consts.TRACE_ID: c.GetString(consts.TRACE_ID),
			})
			return
		}
		tenant = strings.TrimSpace(tenant)
		if tenant == "" {
			tenant = rbac.DefaultTenant
		}
		c.Set(consts.CTX_TENANT_ID, tenant)

		if allow, err = rbac.RBAC().Authorize(c.Request.Context(), tenant, sub, obj, act); err != nil {
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
			logAuthzDecision(c, tenant, sub, obj, act, consts.EffectAllow)
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":          -1,
			"msg":           "permission denied",
			"data":          nil,
			consts.TRACE_ID: c.GetString(consts.TRACE_ID),
		})
		logAuthzDecision(c, tenant, sub, obj, act, consts.EffectDeny)
	}
}

// logAuthzDecision writes one authorization decision to the authz log.
// Every decision shares this single field set, so entries stay correlatable
// by trace_id no matter which branch rejected or admitted the request.
func logAuthzDecision(c *gin.Context, tenant, sub, obj, act string, effect consts.Effect) {
	if logger.Authz == nil {
		return
	}

	logger.Authz.Infoz(
		"",
		zap.String("tenant", tenant),
		zap.String("sub", sub),
		zap.String("obj", obj),
		zap.String("act", act),
		zap.String("eft", string(effect)),
		zap.String("username", c.GetString(consts.CTX_USERNAME)),
		zap.String("trace_id", c.GetString(consts.TRACE_ID)),
	)
}

func resolveAuthzTenant(c *gin.Context, resolver TenantResolver) (string, error) {
	if resolver == nil {
		resolver = currentAuthzTenantResolver()
	}
	return resolver(c)
}

func currentAuthzTenantResolver() TenantResolver {
	authzTenantResolver.RLock()
	defer authzTenantResolver.RUnlock()
	if authzTenantResolver.resolver == nil {
		return defaultTenantResolver
	}
	return authzTenantResolver.resolver
}

func defaultTenantResolver(c *gin.Context) (string, error) {
	if c == nil {
		return rbac.DefaultTenant, nil
	}
	return strings.TrimSpace(c.GetString(consts.CTX_TENANT_ID)), nil
}
