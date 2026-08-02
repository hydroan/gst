package rbac

import (
	"context"
	"strings"

	"github.com/hydroan/gst/types/consts"
)

// AssignRole assigns subject to role inside tenant.
func (r *rbac) AssignRole(ctx context.Context, tenant string, subject string, role string) (err error) {
	tenant = normalizeTenant(tenant)
	if subject == role {
		return nil
	}
	ctx, finishSpan := traceRBAC(ctx, "assign_role", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(ctx, addRules("g", "g", []string{subject, role, tenant}))
}

// UnassignRole removes a subject-role assignment from tenant.
func (r *rbac) UnassignRole(ctx context.Context, tenant string, subject string, role string) (err error) {
	tenant = normalizeTenant(tenant)
	ctx, finishSpan := traceRBAC(ctx, "unassign_role", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(ctx, removeRules("g", "g", []string{subject, role, tenant}))
}

// RolesForSubject returns the roles subject holds inside tenant.
//
// The grouping rules are (subject, role, tenant), so the subject's own rules
// are read and narrowed to the tenant asked about. A rule with an empty role is
// skipped: it names no role to return and the loader would have rejected it,
// so its presence means the table was written around the framework.
func (r *rbac) RolesForSubject(ctx context.Context, tenant string, subject string) ([]string, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, nil
	}
	tenant = normalizeTenant(tenant)

	r.mu.RLock()
	defer r.mu.RUnlock()

	groupingPolicies, err := r.enforcer.GetFilteredGroupingPolicy(0, subject)
	if err != nil {
		return nil, err
	}

	roles := make([]string, 0, len(groupingPolicies))
	for _, policy := range groupingPolicies {
		if len(policy) < 3 || policy[2] != tenant {
			continue
		}
		if role := strings.TrimSpace(policy[1]); role != "" {
			roles = append(roles, role)
		}
	}
	return roles, nil
}

// SubjectsInTenant returns subjects with at least one role assignment inside tenant.
//
// It is used by IAM admin user list because IAM users do not store tenant_id.
// The tenant-visible user set is therefore derived from role bindings first and
// then joined back to user rows by subject ID.
func (r *rbac) SubjectsInTenant(ctx context.Context, tenant string) ([]string, error) {
	tenant = normalizeTenant(tenant)

	r.mu.RLock()
	defer r.mu.RUnlock()

	groupingPolicies, err := r.enforcer.GetFilteredGroupingPolicy(2, tenant)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(groupingPolicies))
	subjects := make([]string, 0, len(groupingPolicies))
	for _, policy := range groupingPolicies {
		if len(policy) < 3 || strings.TrimSpace(policy[1]) == "" {
			continue
		}
		subject := strings.TrimSpace(policy[0])
		if subject == "" {
			continue
		}
		if _, ok := seen[subject]; ok {
			continue
		}
		seen[subject] = struct{}{}
		subjects = append(subjects, subject)
	}
	return subjects, nil
}

// AssignSystemRole assigns a subject to a system-level role outside any tenant.
func (r *rbac) AssignSystemRole(ctx context.Context, subject string, role string) (err error) {
	if subject == role {
		return nil
	}
	ctx, finishSpan := traceRBAC(ctx, "assign_system_role", rbacTraceFields("", role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(ctx, addRules("g", systemRoleGrouping, []string{subject, role}))
}

// UnassignSystemRole removes a subject's system-level role assignment.
func (r *rbac) UnassignSystemRole(ctx context.Context, subject string, role string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "unassign_system_role", rbacTraceFields("", role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(ctx, removeRules("g", systemRoleGrouping, []string{subject, role}))
}

// HasSystemRole reports whether subject explicitly holds a system-level role.
func (r *rbac) HasSystemRole(ctx context.Context, subject string, role string) (bool, error) {
	if subject == role {
		return false, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enforcer.HasNamedGroupingPolicy(systemRoleGrouping, subject, role)
}

func isBuiltInSystemRole(subject string, role string) bool {
	return subject == consts.AUTHZ_USER_ROOT && role == consts.AUTHZ_SYSTEM_ROLE_ROOT
}

// RemoveSubject removes every tenant-scoped and system-level role assignment
// held by subject, across all tenants.
func (r *rbac) RemoveSubject(ctx context.Context, subject string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "remove_subject", nil)
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(
		ctx,
		removeFiltered("g", "g", 0, subject),
		removeFiltered("g", systemRoleGrouping, 0, subject),
	)
}
