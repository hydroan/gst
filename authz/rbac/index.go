package rbac

import (
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/hydroan/gst/types/consts"
)

// decisionIndex is what the policy branches of a decision answer from: the
// stored allow rules, arranged the way a request asks for them.
//
// The matcher in modelData remains the specification of what these lookups
// mean, but it is no longer what executes. The engine evaluated it once per
// stored rule, every rule in the deployment on every request, at an
// interpreter's price per row; the index holds the same rules partitioned by
// what the matcher's cheap comparisons would have rejected — the tenant and
// role equalities — so a decision touches only the rules that could possibly
// allow it: the authenticated set, and the sets of the roles the subject
// holds. The cost of a decision follows the subject's own grant surface
// instead of the size of the deployment.
//
// Only rules whose effect is allow are held. The effect expression is
// some(where allow): a rule carrying any other effect can never allow a
// request, and there is no deny to overrule with, so such a rule decides
// nothing and is not worth a lookup.
//
// An index is immutable once built. Every change to the policy set — a
// mutation batch, a reload, an install — builds a fresh one from the model
// under the enforcer write lock, whole rather than incrementally: policy
// writes happen at administrative frequency, a rebuild is linear in the rule
// count, and a derivation with no update paths has no update bugs.
type decisionIndex struct {
	// authenticated holds the rules of the implicit authenticated role, which
	// the matcher applies to every subject without a tenant check — whatever
	// tenant column such a row carries.
	authenticated []indexedRule

	// byTenantRole holds every other rule under the tenant and role that must
	// both match for the rule to apply.
	byTenantRole map[string]map[string][]indexedRule
}

// indexedRule is one stored allow rule, with the two values a lookup compares
// lifted out of the row.
type indexedRule struct {
	// rule is the policy row as the model holds it, reported as the matched
	// rule of a decision it allows. Rows are never edited in place, so holding
	// the slice is safe; Authorize clones it before handing it to a caller.
	rule []string

	template string
	action   string
}

// policyIndex is the index decisions currently answer from. It is written
// under the enforcer write lock wherever the policy set changes, and read
// under the read lock a decision already holds. Nil means no policy set is
// installed, which decides nothing.
var policyIndex *decisionIndex

// rebuildPolicyIndex derives a fresh index from m and installs it. The caller
// holds the enforcer write lock, which is what keeps the index and the policy
// set it is derived from changing as one.
func rebuildPolicyIndex(m casbinmodel.Model) {
	policyIndex = buildDecisionIndex(m)
}

// buildDecisionIndex derives an index from the p rules m holds, in the order
// the model holds them, which is the order the engine matched them in.
func buildDecisionIndex(m casbinmodel.Model) *decisionIndex {
	index := &decisionIndex{byTenantRole: make(map[string]map[string][]indexedRule)}
	section, ok := m["p"]
	if !ok {
		return index
	}
	ast, ok := section["p"]
	if !ok {
		return index
	}

	for _, rule := range ast.Policy {
		// A rule is (tenant, role, obj, act, eft); the loader sizes every rule
		// to its assertion, so a short one cannot come from storage. Skipping
		// is still the right answer for one that arrives another way: a rule
		// that cannot be compared cannot allow.
		if len(rule) < 5 || rule[4] != string(consts.EffectAllow) {
			continue
		}
		indexed := indexedRule{rule: rule, template: rule[2], action: rule[3]}
		if rule[1] == consts.AUTHZ_ROLE_AUTHENTICATED {
			index.authenticated = append(index.authenticated, indexed)
			continue
		}
		roles, ok := index.byTenantRole[rule[0]]
		if !ok {
			roles = make(map[string][]indexedRule)
			index.byTenantRole[rule[0]] = roles
		}
		roles[rule[1]] = append(roles[rule[1]], indexed)
	}
	return index
}

// matchRules reports the first rule in rules that covers the request, in the
// order the model holds them.
//
// The action is compared before the template: methods are few, the comparison
// is a handful of nanoseconds against a cached regexp's hundreds, and most
// rules fail on it. That order also narrows what an uncompilable template can
// break — only requests whose action reaches it — which is one step further
// than the partitioning already went: the engine evaluated every template in
// the deployment on every denial, so one bad row failed every request, while
// here it fails only the requests it could have allowed.
func matchRules(rules []indexedRule, object string, action string) (bool, []string, error) {
	for i := range rules {
		if rules[i].action != action {
			continue
		}
		matched, err := pathMatch(object, rules[i].template)
		if err != nil {
			return false, nil, err
		}
		if matched {
			return true, rules[i].rule, nil
		}
	}
	return false, nil, nil
}
