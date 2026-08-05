package serviceauthz

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types/consts"
)

// PolicyDrift is one stored authorization rule that the records it should have
// come from no longer justify, or one such record whose rule is missing.
type PolicyDrift struct {
	// Kind is "permission" for a role's route grant and "binding" for a
	// subject's role assignment.
	Kind string

	// Direction is "orphaned" when the rule is stored but no record derives it,
	// and "missing" when a record derives it but no rule is stored.
	Direction string

	Tenant string
	Role   string

	// Subject is set for a binding, Object and Action for a permission.
	Subject string
	Object  string
	Action  string
}

// PolicyReport is the outcome of comparing stored authorization rules against
// the records they are derived from.
type PolicyReport struct {
	// Drifts lists every disagreement found, orphaned rules first.
	Drifts []PolicyDrift

	// Checked counts the stored rules the comparison covered, so an empty
	// Drifts list can be told apart from a comparison that examined nothing.
	Checked int

	// Skipped counts stored rules the comparison deliberately ignored because
	// no record derives them. See ReconcilePolicies for which those are.
	Skipped int
}

// InSync reports whether the comparison found no disagreement.
func (r PolicyReport) InSync() bool { return len(r.Drifts) == 0 }

// ReconcilePolicies compares the stored authorization rules against the records
// they are derived from and reports the disagreements.
//
// Roles, their selected menus, and role bindings are the source; the policy
// table is a projection of them maintained as they change. The projection can
// fall behind when something writes it outside that path — another replica
// while this one was down, a manual repair, a restore from backup, or an
// after-commit update that failed once the transaction was already durable.
//
// It reports and does not repair. An orphaned rule may equally be a projection
// bug or a record deleted out from under a rule that is still wanted, and
// deleting rules on that guess risks removing access nobody asked to remove.
// Repair is a decision for whoever reads the report.
//
// Two kinds of stored rule are counted in Skipped rather than compared, because
// nothing in the records derives them: policies written against the implicit
// authenticated role, which the application declares directly, and system-level
// role assignments, which live outside any tenant. Treating either as orphaned
// would report the framework's own baseline as drift on every run.
//
// The comparison is between stored data only: the policy table against the
// records it is derived from. It does not look at the in-memory decision state
// any process holds, so its answer is the same from every replica and stays
// meaningful on one whose memory has drifted.
func ReconcilePolicies(ctx context.Context) (PolicyReport, error) {
	// The stored rules cover every tenant, so the records they are compared
	// against have to as well. Left scoped, the comparison would read one
	// tenant's records against every tenant's rules and report the rest as
	// orphaned — a report that is wrong in the direction of deleting access.
	ctx = tenant.Across(ctx)

	report := PolicyReport{Drifts: make([]PolicyDrift, 0)}

	expected, err := expectedPolicies(ctx)
	if err != nil {
		return report, err
	}

	stored := make([]*modelauthz.AuthzRule, 0)
	if err := database.Database[*modelauthz.AuthzRule](ctx).List(&stored); err != nil {
		return report, err
	}

	seen := make(map[string]struct{}, len(stored))
	for _, rule := range stored {
		if skipReconcile(rule) {
			report.Skipped++
			continue
		}
		report.Checked++

		key := policyKey(rule.Ptype, rule.V0, rule.V1, rule.V2, rule.V3)
		seen[key] = struct{}{}
		if _, ok := expected[key]; !ok {
			report.Drifts = append(report.Drifts, driftFromRule(rule, "orphaned"))
		}
	}

	// Sorted so that two runs over the same data report the same order, which
	// is what makes the report diffable between runs.
	for _, key := range slices.Sorted(maps.Keys(expected)) {
		if _, ok := seen[key]; !ok {
			report.Drifts = append(report.Drifts, expected[key])
		}
	}
	return report, nil
}

// expectedPolicies derives the rules the records call for, keyed the same way
// the stored ones are.
func expectedPolicies(ctx context.Context) (map[string]PolicyDrift, error) {
	roles := make([]*modelauthz.Role, 0)
	if err := database.Database[*modelauthz.Role](ctx).List(&roles); err != nil {
		return nil, err
	}
	menus := make([]*modelauthz.Menu, 0)
	if err := database.Database[*modelauthz.Menu](ctx).List(&menus); err != nil {
		return nil, err
	}
	bindings := make([]*modelauthz.RoleBinding, 0)
	if err := database.Database[*modelauthz.RoleBinding](ctx).List(&bindings); err != nil {
		return nil, err
	}

	byMenuID := make(map[string]*modelauthz.Menu, len(menus))
	for _, menu := range menus {
		byMenuID[menu.ID] = menu
	}

	expected := make(map[string]PolicyDrift)
	for _, role := range roles {
		tenant := reconcileTenant(string(role.TenantID))
		for _, menuID := range role.MenuIDs {
			menu, ok := byMenuID[menuID]
			if !ok {
				continue
			}
			for _, permission := range modelauthz.RoutePermissionsForMenu(menu) {
				key := policyKey("p", tenant, role.ID, permission.Object, permission.Action)
				expected[key] = PolicyDrift{
					Kind: "permission", Direction: "missing",
					Tenant: tenant, Role: role.ID,
					Object: permission.Object, Action: permission.Action,
				}
			}
		}
	}
	for _, binding := range bindings {
		tenant := reconcileTenant(string(binding.TenantID))
		key := policyKey("g", binding.SubjectID, binding.RoleID, tenant, "")
		expected[key] = PolicyDrift{
			Kind: "binding", Direction: "missing",
			Tenant: tenant, Role: binding.RoleID, Subject: binding.SubjectID,
		}
	}
	return expected, nil
}

// skipReconcile reports whether a stored rule has no counterpart among the
// records and must therefore not be judged against them.
func skipReconcile(rule *modelauthz.AuthzRule) bool {
	switch rule.Ptype {
	case "p":
		// Declared by the application through SetPermissionsForAuthenticated,
		// not derived from any role.
		return rule.V1 == consts.AUTHZ_ROLE_AUTHENTICATED
	case "g":
		// The built-in tenant administrator role is granted by assignment
		// alone: it has no row in the roles table, so no role binding can name
		// it and nothing among the records derives it. Judging it against them
		// reports the framework's own baseline as drift on every run.
		return rule.V1 == consts.AUTHZ_ROLE_ADMIN
	default:
		// g2 and anything else. A system-level assignment is written straight
		// through AssignSystemRole by whatever bootstraps the deployment, so
		// there is no record to derive it from — which also means this
		// comparison cannot see a system role somebody granted themselves.
		return true
	}
}

func driftFromRule(rule *modelauthz.AuthzRule, direction string) PolicyDrift {
	if rule.Ptype == "g" {
		return PolicyDrift{
			Kind: "binding", Direction: direction,
			Tenant: rule.V2, Role: rule.V1, Subject: rule.V0,
		}
	}
	return PolicyDrift{
		Kind: "permission", Direction: direction,
		Tenant: rule.V0, Role: rule.V1, Object: rule.V2, Action: rule.V3,
	}
}

// reconcileTenant names the domain a record's rules were written under.
//
// It has to normalize exactly the way the write did, or the comparison keys
// disagree with the rules they are meant to match: a record whose tenant
// carries surrounding space has its rules stored trimmed, and looking for them
// untrimmed reports every one of them as orphaned and missing at once.
//
// Nothing reachable writes such a record any more — the tenant package trims at
// every entry — so this stands for rows that predate it or were written around
// the framework, which are exactly the rows a drift report exists to find.
func reconcileTenant(id string) string {
	if id = strings.TrimSpace(id); id != "" {
		return id
	}
	return tenant.Default
}

// policyKey identifies a rule by the columns that decide it, so a stored row
// and a derived expectation for the same rule collide.
func policyKey(parts ...string) string { return strings.Join(parts, "\x00") }
