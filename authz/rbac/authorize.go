package rbac

import (
	"context"
	"slices"

	prommetrics "github.com/hydroan/gst/metrics"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// policyRoleColumn is the position of the role token in a p policy row. The
// policy definition is (tenant, role, obj, act, eft), so a row Casbin reports as
// the matched one carries its role here.
const policyRoleColumn = 1

// Authorize evaluates the request and reports which rule allowed it.
//
// It is the one method on the hot path, so it is also the one worth watching:
// every request makes exactly one call, and without a span of its own the time
// it takes is spread invisibly across whatever encloses it. The span and the
// counter both cost nothing when tracing is off and metrics were never
// initialized, which is what a decision path can afford.
func (r *rbac) Authorize(
	ctx context.Context, tenant string, subject string, object string, action string,
) (types.Decision, error) {
	tenant = normalizeTenant(tenant)

	// The span context goes nowhere on purpose: nothing below this opens a span
	// of its own, so this is a leaf and there is no child to parent.
	finishSpan := traceAuthorize(ctx, tenant)

	r.mu.RLock()
	defer r.mu.RUnlock()

	allowed, source, matchedRule, err := r.authorize(tenant, subject, object, action)
	// The rule is copied out. What the engine reports is its own storage, and
	// handing that to a caller makes every consumer of a decision one edit away
	// from rewriting the policy set it was decided from.
	decision := types.Decision{Allowed: allowed, Source: source, MatchedRule: slices.Clone(matchedRule)}
	if err == nil && !allowed {
		decision.Reason = r.denyReason(tenant, subject)
	}

	finishSpan(decision, err)
	countDecision(decision, err)
	return decision, err
}

// denyReason names what a denied request is missing.
//
// Every grant needs two things: the subject holds a role here, and that role
// carries a permission covering the request. A denial means one of them is
// absent, and which one decides where the repair goes — a role binding, or a
// permission. The request tuple in the log says neither.
//
// It runs on a denial only, so no allowed request pays for it. The role graph
// is asked rather than the stored rules, which is the same source the decision
// itself was taken from; direct membership is enough to tell the two apart,
// because a subject reaching a role through another one holds that other one
// directly.
//
// An unknown answer is reported as no reason at all. Guessing between the two
// would send an operator to repair the step that was never broken.
func (r *rbac) denyReason(tenant string, subject string) consts.DenyReason {
	manager := r.enforcer.GetNamedRoleManager(tenantRoleGrouping)
	if manager == nil {
		// No grouping means no subject holds a role, whatever is stored.
		return consts.DenyReasonNoRole
	}
	roles, err := manager.GetRoles(subject, tenant)
	if err != nil {
		return ""
	}
	if len(roles) == 0 {
		return consts.DenyReasonNoRole
	}
	return consts.DenyReasonNoPolicy
}

// authorize decides the request and names the rule that allowed it.
//
// The two role branches are answered here rather than in the matcher because
// neither consults a policy. The matcher is evaluated once per stored policy
// row, so a branch that ignores p is a constant the engine recomputes for every
// row, and a deployment pays for it on every request. Answering them here also
// leaves one copy of the precedence: it used to live in the matcher and again
// in the derivation below it, with nothing keeping the two in step.
//
// The order is load-bearing. A subject can satisfy several branches at once,
// and naming a weaker one would suggest that revoking it takes the access away:
// a system_root subject that also holds a role granting the same route must be
// reported as system_root, or an operator who then strips the role will find
// the route still reachable and no explanation for it.
//
// Only the policy branches yield a matched rule. The two above them never
// consult a policy, so there is no row that is the reason for access.
//
// The read lock belongs to the caller and has to cover the whole of this: the
// branches and the enforcement have to see one policy set, and taking the lock
// here would deadlock the exported methods that already hold it.
func (r *rbac) authorize(
	tenant string, subject string, object string, action string,
) (bool, consts.GrantSource, []string, error) {
	systemRoot, err := r.hasRoleLink(systemRoleGrouping, subject, consts.AUTHZ_SYSTEM_ROLE_ROOT)
	if err != nil {
		return false, "", nil, err
	}
	if systemRoot {
		return true, consts.GrantSourceSystemRoot, nil, nil
	}

	tenantAdmin, err := r.hasRoleLink(tenantRoleGrouping, subject, consts.AUTHZ_ROLE_ADMIN, tenant)
	if err != nil {
		return false, "", nil, err
	}
	if tenantAdmin {
		return true, consts.GrantSourceTenantAdmin, nil, nil
	}

	allowed, matchedRule, err := r.enforcer.EnforceEx(tenant, subject, object, action)
	if err != nil || !allowed {
		return allowed, "", nil, err
	}
	if len(matchedRule) > policyRoleColumn && matchedRule[policyRoleColumn] == consts.AUTHZ_ROLE_AUTHENTICATED {
		return allowed, consts.GrantSourceAuthenticated, matchedRule, nil
	}
	return allowed, consts.GrantSourceRole, matchedRule, nil
}

// hasRoleLink reports whether subject reaches role through the grouping ptype,
// answering exactly what the g function in the matcher would have answered.
//
// The role manager is asked rather than the stored rules. It resolves a subject
// that reaches the role through another role, which a lookup of the rules as
// written does not, and moving a branch out of the matcher must not change what
// that branch decides.
//
// The inequality is part of that agreement: HasLink reports a self-match, so a
// subject named after the role would otherwise be handed it. The matcher
// guarded against that with the same test.
func (r *rbac) hasRoleLink(ptype string, subject string, role string, domain ...string) (bool, error) {
	if subject == role {
		return false, nil
	}
	manager := r.enforcer.GetNamedRoleManager(ptype)
	if manager == nil {
		return false, nil
	}
	return manager.HasLink(subject, role, domain...)
}

// countDecision publishes one decision to whoever is watching.
//
// An error is neither an allow nor a deny: policy never decided it, and folding
// it into either effect would move the counts the other one is read for. Each
// effect carries only the explanation that exists for it — a rule kind for an
// allow, a missing step for a denial — and leaves the other label empty, so a
// series always answers the question its effect can answer.
func countDecision(decision types.Decision, err error) {
	// The counter is created by prommetrics.Init, which bootstrap runs long
	// before the first request. A process that never ran bootstrap, which is
	// what a test is, still has to be able to decide.
	if prommetrics.AuthzDecisionsTotal == nil {
		return
	}
	switch {
	case err != nil:
		prommetrics.AuthzDecisionsTotal.WithLabelValues("error", "", "").Inc()
	case decision.Allowed:
		prommetrics.AuthzDecisionsTotal.
			WithLabelValues(string(consts.EffectAllow), string(decision.Source), "").Inc()
	default:
		prommetrics.AuthzDecisionsTotal.
			WithLabelValues(string(consts.EffectDeny), "", string(decision.Reason)).Inc()
	}
}
