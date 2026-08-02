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
	"github.com/hydroan/gst/tenant"
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
		tenantID, err := resolveAuthzTenant(c, cfg.TenantResolver)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":          -1,
				"msg":           "authorization failed",
				"data":          nil,
				consts.TRACE_ID: c.GetString(consts.TRACE_ID),
			})
			// The tenant is unknown at this point, so the attempt is recorded
			// without one.
			logAuthzFailure(c, "", sub, obj, act, err)
			return
		}
		tenantID = strings.TrimSpace(tenantID)
		if tenantID == "" {
			tenantID = tenant.Default
		}
		c.Set(consts.CTX_TENANT_ID, tenantID)

		var source consts.GrantSource
		var matchedRule []string
		if allow, source, matchedRule, err = rbac.RBAC().
			AuthorizeExplained(c.Request.Context(), tenantID, sub, obj, act); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":          -1,
				"msg":           "authorization failed",
				"data":          nil,
				consts.TRACE_ID: c.GetString(consts.TRACE_ID),
			})
			logAuthzFailure(c, tenantID, sub, obj, act, err)
			return
		}
		if allow {
			// A subject allowed as system_root was not authorized in any one
			// tenant — the matcher grants it every object in every tenant — so
			// binding its rows to one would leave the two halves disagreeing:
			// allowed to see everything, shown a slice. The scope is taken from
			// the authorization decision itself rather than looked up again,
			// which is what keeps the reach of the data equal to the reach of
			// the grant.
			if source == consts.GrantSourceSystemRoot {
				c.Request = c.Request.WithContext(tenant.Across(c.Request.Context()))
			}
			logAuthzGrant(c, tenantID, sub, obj, act, source, matchedRule)
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":          -1,
			"msg":           "permission denied",
			"data":          nil,
			consts.TRACE_ID: c.GetString(consts.TRACE_ID),
		})
		logAuthzDecision(c, tenantID, sub, obj, act, consts.EffectDeny)
	}
}

// logAuthzGrant writes one allowed authorization decision, together with what
// allowed it.
//
// The two extra fields answer a question the request tuple alone cannot: several
// rules may permit the same request, so "allowed" on its own does not say which
// grant to revoke to take the access away. allowed_by names the rule kind and is
// low-cardinality enough to aggregate over; matched_rule carries the policy row
// and is present only when a policy is what allowed the request.
//
// matched_rule is worth recording next to obj because the two differ: obj is the
// concrete path of this request, while the rule holds the pattern that matched
// it, such as /api/things/{id}.
func logAuthzGrant(
	c *gin.Context, tenant, sub, obj, act string, source consts.GrantSource, matchedRule []string,
) {
	if logger.Authz == nil {
		return
	}

	fields := append(
		authzLogFields(c, tenant, sub, obj, act),
		zap.String("eft", string(consts.EffectAllow)),
		zap.String("allowed_by", string(source)),
	)
	if len(matchedRule) > 0 {
		fields = append(fields, zap.Strings("matched_rule", matchedRule))
	}
	logger.Authz.Infoz("", fields...)
}

// logAuthzDecision writes one completed authorization decision to the authz log.
// It is called at decision time, before the handler chain runs, so timestamps
// mean the same thing for every effect and a panicking handler cannot drop
// an allowed decision.
func logAuthzDecision(c *gin.Context, tenant, sub, obj, act string, effect consts.Effect) {
	if logger.Authz == nil {
		return
	}

	logger.Authz.Infoz("", append(
		authzLogFields(c, tenant, sub, obj, act),
		zap.String("eft", string(effect)),
	)...)
}

// logAuthzFailure writes an authorization attempt that could not be decided.
// Such an attempt carries no eft because policy never allowed or denied it:
// reporting one would hide the failure and inflate the counts the other effect
// is used to measure. The error level tells the two apart instead.
func logAuthzFailure(c *gin.Context, tenant, sub, obj, act string, err error) {
	if logger.Authz == nil {
		return
	}

	logger.Authz.Errorz("", append(
		authzLogFields(c, tenant, sub, obj, act),
		zap.Error(err),
	)...)
}

// authzLogFieldCount is the shared field count plus the most any caller appends
// to it: an effect, a grant source, and a matched rule. Reserving it keeps the
// append from growing the slice, which would cost a second allocation and a copy
// on every authorized request.
const authzLogFieldCount = 9

// authzLogFields builds the field set shared by every authz log entry, so
// entries stay correlatable by trace_id no matter which branch produced them.
func authzLogFields(c *gin.Context, tenant, sub, obj, act string) []zap.Field {
	return append(
		make([]zap.Field, 0, authzLogFieldCount),
		zap.String("tenant", tenant),
		zap.String("sub", sub),
		zap.String("obj", obj),
		zap.String("act", act),
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
		return tenant.Default, nil
	}
	return strings.TrimSpace(c.GetString(consts.CTX_TENANT_ID)), nil
}
