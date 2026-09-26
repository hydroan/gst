package rbac

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// Subject is who asks for an action, as the transport established it: the
// HTTP middleware reads it off the gin context, the gRPC interceptor off the
// call's caller.
type Subject struct {
	UserID   string
	Username string
	TenantID string
}

// Enforce decides whether subject may perform act on obj — the HTTP method
// and the path or route the action is served at — the one decision path
// both the HTTP middleware and the gRPC interceptor of the authz module
// take, and logs the decision (see logGrant, logDeny and logFailure) under
// the trace id ctx carries, the one both listeners stamp on it. An
// anonymous subject is refused before any decision. The tenant is
// subject's, tenant.Default when it has none, and comes back as the one the
// decision was made in, for the transport to publish onward; it is trusted
// twice over — the decision is made in it and every tenant-scoped row the
// action then reads or writes is scoped to it — so a transport must only
// ever establish it from something the deployment vouches for, never from
// client input passed through as it stands. A subject allowed as
// system_root was not authorized in any one tenant, so the returned context
// reaches across tenants (see tenant.Across), which keeps the reach of the
// data equal to the reach of the grant.
//
// A refusal is a service error carrying the status and message the client
// is answered with: 403 "permission denied" for an anonymous subject and a
// denied one, 500 "authorization unavailable" when the decision could not
// be made, which is this server's failure and not the client's — nothing
// about the request is wrong, and the client cannot change anything to make
// the decision reachable.
func Enforce(ctx context.Context, subject Subject, obj, act string) (context.Context, string, error) {
	// Every entry reports how long deciding took. Deciding is in-memory work
	// over the whole policy set, so it appears in no other log: the access
	// log's duration contains it without separating it, and the policy set
	// is only read from the database when it is reloaded. Left unmeasured, a
	// policy set growing until enforcement dominates every request would be
	// invisible. The measure covers the refusal paths too, so none is
	// silently exempt from it.
	start := time.Now()

	sub := strings.TrimSpace(subject.UserID)
	if sub == "" {
		// Anonymous subjects are refused before the tenant is resolved, so
		// the decision is recorded without one.
		logDeny(ctx, subject, "", sub, obj, act, consts.DenyReasonUnauthenticated, time.Since(start))
		return ctx, "", serviceregistry.NewError(http.StatusForbidden, "permission denied")
	}
	tenantID := strings.TrimSpace(subject.TenantID)
	if tenantID == "" {
		tenantID = tenant.Default
	}

	decision, err := RBAC().Authorize(ctx, tenantID, sub, obj, act)
	if err != nil {
		logFailure(ctx, subject, tenantID, sub, obj, act, err, time.Since(start))
		return ctx, tenantID, serviceregistry.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if !decision.Allowed {
		logDeny(ctx, subject, tenantID, sub, obj, act, decision.Reason, time.Since(start))
		return ctx, tenantID, serviceregistry.NewError(http.StatusForbidden, "permission denied")
	}
	// The scope is taken from the decision itself rather than looked up
	// again, which is what keeps the reach of the data equal to the reach
	// of the grant.
	if decision.Source == consts.GrantSourceSystemRoot {
		ctx = tenant.Across(ctx)
	}
	logGrant(ctx, subject, tenantID, sub, obj, act, decision.Source, decision.MatchedRule, time.Since(start))
	return ctx, tenantID, nil
}

// logGrant writes one allowed decision, together with what allowed it.
//
// The two extra fields answer a question the request tuple alone cannot:
// several rules may permit the same request, so "allowed" on its own does
// not say which grant to revoke to take the access away. allowed_by names
// the rule kind and is low-cardinality enough to aggregate over;
// matched_rule carries the policy row and is present only when a policy is
// what allowed the request. It is worth recording next to obj because the
// two differ: obj is the concrete path of this request, while the rule holds
// the pattern that matched it, such as /api/things/{id}.
func logGrant(ctx context.Context, subject Subject, tenantID, sub, obj, act string, source consts.GrantSource, matchedRule []string, elapsed time.Duration) {
	if logger.Authz == nil {
		return
	}
	fields := append(
		enforceLogFields(ctx, subject, tenantID, sub, obj, act, elapsed),
		zap.String("eft", string(consts.EffectAllow)),
		zap.String("allowed_by", string(source)),
	)
	if len(matchedRule) > 0 {
		fields = append(fields, zap.Strings("matched_rule", matchedRule))
	}
	logger.Authz.Infoz("", fields...)
}

// logDeny writes one refused request, together with what it was missing.
//
// denied_by is the mirror of allowed_by: a denial names no rule, so the only
// thing it can report is which of the two steps behind a grant did not
// happen — the subject holding a role here, or a permission covering the
// request. The two lead to opposite repairs, and the request tuple beside
// it says neither. It is omitted when nothing could be determined, so an
// absent field reads as unknown rather than as a reason of its own. It is
// written at decision time, before the action runs, so timestamps mean the
// same thing for every effect and a panicking handler cannot drop a
// decision.
func logDeny(ctx context.Context, subject Subject, tenantID, sub, obj, act string, reason consts.DenyReason, elapsed time.Duration) {
	if logger.Authz == nil {
		return
	}
	fields := append(
		enforceLogFields(ctx, subject, tenantID, sub, obj, act, elapsed),
		zap.String("eft", string(consts.EffectDeny)),
	)
	if reason != "" {
		fields = append(fields, zap.String("denied_by", string(reason)))
	}
	logger.Authz.Infoz("", fields...)
}

// logFailure writes an authorization attempt that could not be decided.
// Such an attempt carries no eft because policy never allowed or denied it:
// reporting one would hide the failure and inflate the counts the other
// effect is used to measure. The error level tells the two apart instead.
func logFailure(ctx context.Context, subject Subject, tenantID, sub, obj, act string, err error, elapsed time.Duration) {
	if logger.Authz == nil {
		return
	}
	logger.Authz.Errorz("", append(
		enforceLogFields(ctx, subject, tenantID, sub, obj, act, elapsed),
		zap.Error(err),
	)...)
}

// enforceLogFieldCount is the seven shared fields plus the most any one
// caller appends: a grant adds an effect, a source and a matched rule, which
// is three and more than either other caller. Reserving it keeps the append
// from growing the slice, which would cost a second allocation and a copy
// on every authorized request; a test holds the worst case to it exactly.
// util.LogDuration counts as one of the seven, rendering two keys from a
// single inlined field.
const enforceLogFieldCount = 10

// enforceLogFields builds the field set shared by every entry, so entries
// stay correlatable by the trace id ctx carries no matter which branch
// produced them, and so the elapsed time is reported the same way on all of
// them.
func enforceLogFields(ctx context.Context, subject Subject, tenantID, sub, obj, act string, elapsed time.Duration) []zap.Field {
	return append(
		make([]zap.Field, 0, enforceLogFieldCount),
		zap.String("tenant", tenantID),
		zap.String("sub", sub),
		zap.String("obj", obj),
		zap.String("act", act),
		zap.String("username", subject.Username),
		zap.String(consts.TRACE_ID, execctx.FromContext(ctx).TraceID),
		util.LogDuration(elapsed),
	)
}
