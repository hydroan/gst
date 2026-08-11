package middleware

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/response"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// Authz authorizes requests using RBAC.
// It derives subject from trusted request context and blocks anonymous requests.
// Authz must be called before config.Init so config.Init can read
// AUTH_RBAC_ENABLED from the environment and enable RBAC initialization.
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
// IAMSession and before Authz and overwrites it. The value is trusted twice
// over — the authorization decision is made in that tenant, and every
// tenant-scoped row the request then reads or writes is scoped to it — so it
// must only ever be written from something the deployment vouches for, never
// from client input passed through as it stands: policies for the implicit
// authenticated role match in every tenant, so a request naming a forged
// tenant would pass those and act on that tenant's rows.
func Authz() gin.HandlerFunc {
	os.Setenv(config.AUTH_RBAC_ENABLED, "true")

	return func(c *gin.Context) {
		// Every entry reports how long this middleware took to settle the
		// request. Deciding is in-memory work over the whole policy set, so it
		// appears in no other log: the access log's duration contains it without
		// separating it, and the policy set is only read from the database when
		// it is reloaded, so no statement is logged per request either. Left
		// unmeasured, a policy set growing until enforcement dominates every
		// request would be invisible.
		//
		// The measure is the middleware, not the Authorize call, so that one
		// refusal path is not silently exempt from it. It runs to just before
		// each entry is written, and so covers writing the refusal body on the
		// paths that refuse. Anonymous requests never reach Authorize and their
		// durations are correspondingly tiny; denied_by separates them.
		start := time.Now()

		obj := c.Request.URL.Path
		act := c.Request.Method

		sub := strings.TrimSpace(c.GetString(consts.CTX_USER_ID))
		if sub == "" {
			response.Abort(c, http.StatusForbidden, "permission denied")
			// Anonymous requests are rejected before the tenant is resolved,
			// so the decision is recorded without one.
			logAuthzDeny(c, "", sub, obj, act, consts.DenyReasonUnauthenticated, time.Since(start))
			return
		}
		tenantID := strings.TrimSpace(c.GetString(consts.CTX_TENANT_ID))
		if tenantID == "" {
			tenantID = tenant.Default
		}
		c.Set(consts.CTX_TENANT_ID, tenantID)

		// An attempt that could not be decided is reported as this server's
		// failure, which is what it is: nothing about the request is wrong, and
		// the client cannot change anything to make the decision reachable.
		// Answering 400 filed authorization outages under client error, where
		// nothing watching for server faults would ever see them.
		decision, err := rbac.RBAC().
			Authorize(c.Request.Context(), tenantID, sub, obj, act)
		if err != nil {
			response.Abort(c, http.StatusInternalServerError, "authorization unavailable")
			logAuthzFailure(c, tenantID, sub, obj, act, err, time.Since(start))
			return
		}
		if decision.Allowed {
			// A subject allowed as system_root was not authorized in any one
			// tenant — the matcher grants it every object in every tenant — so
			// binding its rows to one would leave the two halves disagreeing:
			// allowed to see everything, shown a slice. The scope is taken from
			// the authorization decision itself rather than looked up again,
			// which is what keeps the reach of the data equal to the reach of
			// the grant.
			if decision.Source == consts.GrantSourceSystemRoot {
				c.Request = c.Request.WithContext(tenant.Across(c.Request.Context()))
			}
			logAuthzGrant(c, tenantID, sub, obj, act, decision.Source, decision.MatchedRule, time.Since(start))
			c.Next()
			return
		}
		response.Abort(c, http.StatusForbidden, "permission denied")
		logAuthzDeny(c, tenantID, sub, obj, act, decision.Reason, time.Since(start))
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
	c *gin.Context, tenant, sub, obj, act string,
	source consts.GrantSource, matchedRule []string, elapsed time.Duration,
) {
	if logger.Authz == nil {
		return
	}

	fields := append(
		authzLogFields(c, tenant, sub, obj, act, elapsed),
		zap.String("eft", string(consts.EffectAllow)),
		zap.String("allowed_by", string(source)),
	)
	if len(matchedRule) > 0 {
		fields = append(fields, zap.Strings("matched_rule", matchedRule))
	}
	logger.Authz.Infoz("", fields...)
}

// logAuthzDeny writes one refused request, together with what it was missing.
//
// denied_by is the mirror of allowed_by: a denial names no rule, so the only
// thing it can report is which of the two steps behind a grant did not happen —
// the subject holding a role here, or a permission covering the request. The
// two lead to opposite repairs, and the request tuple beside it says neither.
// It is omitted when nothing could be determined, so an absent field reads as
// unknown rather than as a reason of its own.
//
// It is called at decision time, before the handler chain runs, so timestamps
// mean the same thing for every effect and a panicking handler cannot drop a
// decision.
func logAuthzDeny(
	c *gin.Context, tenant, sub, obj, act string, reason consts.DenyReason, elapsed time.Duration,
) {
	if logger.Authz == nil {
		return
	}

	fields := append(
		authzLogFields(c, tenant, sub, obj, act, elapsed),
		zap.String("eft", string(consts.EffectDeny)),
	)
	if reason != "" {
		fields = append(fields, zap.String("denied_by", string(reason)))
	}
	logger.Authz.Infoz("", fields...)
}

// logAuthzFailure writes an authorization attempt that could not be decided.
// Such an attempt carries no eft because policy never allowed or denied it:
// reporting one would hide the failure and inflate the counts the other effect
// is used to measure. The error level tells the two apart instead.
func logAuthzFailure(
	c *gin.Context, tenant, sub, obj, act string, err error, elapsed time.Duration,
) {
	if logger.Authz == nil {
		return
	}

	logger.Authz.Errorz("", append(
		authzLogFields(c, tenant, sub, obj, act, elapsed),
		zap.Error(err),
	)...)
}

// authzLogFieldCount is the seven shared fields plus the most any one caller
// appends: a grant adds an effect, a source and a matched rule, which is three
// and more than either other caller. Reserving it keeps the append from growing
// the slice, which would cost a second allocation and a copy on every
// authorized request. util.LogDuration counts as one of the seven, rendering
// two keys from a single inlined field.
//
// A caller adding a field has to check its own total against this, not raise it
// on sight: a denial appends two and a failure one, so both stay inside a
// reservation sized for the grant.
const authzLogFieldCount = 10

// authzLogFields builds the field set shared by every authz log entry, so
// entries stay correlatable by trace_id no matter which branch produced them,
// and so the elapsed time is reported the same way on all of them.
func authzLogFields(c *gin.Context, tenant, sub, obj, act string, elapsed time.Duration) []zap.Field {
	return append(
		make([]zap.Field, 0, authzLogFieldCount),
		zap.String("tenant", tenant),
		zap.String("sub", sub),
		zap.String("obj", obj),
		zap.String("act", act),
		zap.String("username", c.GetString(consts.CTX_USERNAME)),
		zap.String("trace_id", c.GetString(consts.TRACE_ID)),
		util.LogDuration(elapsed),
	)
}
