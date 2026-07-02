package rbac

import (
	"strings"

	"github.com/casbin/casbin/v3"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// DefaultTenant is the built-in authorization domain used when no tenant
// resolver is configured by the application.
const DefaultTenant = "default"

const systemRoleGrouping = "g2"

var (
	Enforcer *casbin.SyncedEnforcer
	Adapter  *gormadapter.Adapter
)

type rbac struct {
	enforcer *casbin.SyncedEnforcer
	adapter  *gormadapter.Adapter
}

// noop implements RBAC behavior before Casbin is initialized.
// It keeps the built-in root subject as system_root so modules that do not
// register authz can still use root-only administrative flows.
type noop struct{}

func (noop) Authorize(tenant string, subject string, object string, action string) (bool, error) {
	return false, nil
}

func (noop) AddRole(tenant string, role string) error    { return nil }
func (noop) RemoveRole(tenant string, role string) error { return nil }
func (noop) GrantPermission(tenant string, role string, object string, action string) error {
	return nil
}

func (noop) RevokePermission(tenant string, role string, object string, action string) error {
	return nil
}
func (noop) RevokeRolePermissions(tenant string, role string) error        { return nil }
func (noop) AssignRole(tenant string, subject string, role string) error   { return nil }
func (noop) UnassignRole(tenant string, subject string, role string) error { return nil }
func (noop) SubjectInTenant(tenant string, subject string) (bool, error)   { return false, nil }
func (noop) SubjectsInTenant(tenant string) ([]string, error)              { return nil, nil }
func (noop) AssignSystemRole(subject string, role string) error            { return nil }
func (noop) UnassignSystemRole(subject string, role string) error          { return nil }
func (noop) HasSystemRole(subject string, role string) (bool, error) {
	return isBuiltInSystemRole(subject, role), nil
}

func RBAC() types.RBAC {
	// When RBAC is disabled or Enforcer is not initialized,
	// return a safe no-op implementation to prevent panics.
	if Enforcer == nil {
		return noop{}
	}
	return &rbac{
		enforcer: Enforcer,
		adapter:  Adapter,
	}
}

// Authorize evaluates whether subject may perform action on object in tenant.
func (r *rbac) Authorize(tenant string, subject string, object string, action string) (bool, error) {
	return r.enforcer.Enforce(tenant, subject, object, action)
}

// AddRole is a no-op because Casbin creates roles implicitly when a tenant
// receives permissions or grouping policies for that role.
func (r *rbac) AddRole(tenant string, role string) error {
	return nil
}

// RemoveRole removes all policies and subject assignments for role in tenant.
func (r *rbac) RemoveRole(tenant string, role string) error {
	policyErr := r.RevokeRolePermissions(tenant, role)
	_, groupingErr := r.enforcer.RemoveFilteredGroupingPolicy(1, role, tenant)
	return errors.Join(policyErr, groupingErr)
}

// GrantPermission grants role access to object/action inside tenant.
func (r *rbac) GrantPermission(tenant string, role string, object string, action string) error {
	if _, err := r.enforcer.AddPolicy(tenant, role, object, action, string(consts.EffectAllow)); err != nil {
		return err
	}
	return nil
}

// RevokePermission removes the exact tenant, role, object, action permission.
func (r *rbac) RevokePermission(tenant string, role string, object string, action string) error {
	if _, err := r.enforcer.RemovePolicy(tenant, role, object, action, string(consts.EffectAllow)); err != nil {
		return err
	}
	return nil
}

// RevokeRolePermissions removes every permission policy granted to role in tenant.
// It is the explicit form of revoking a role's full permission set. Use
// RevokePermission when removing one concrete object/action grant.
func (r *rbac) RevokeRolePermissions(tenant string, role string) error {
	if _, err := r.enforcer.RemoveFilteredPolicy(0, tenant, role); err != nil {
		return err
	}
	return nil
}

// | Operation                    | Casbin function                                           |
// | ---------------------------- | --------------------------------------------------------- |
// | Grant role permission        | `AddPolicy(tenant, role, obj, act, eft)`                  |
// | Revoke role permission       | `RemovePolicy(tenant, role, obj, act, eft)`               |
// | Revoke all role permissions  | `RemoveFilteredPolicy(0, tenant, role)`                   |
// | Assign role to subject       | `AddGroupingPolicy(subject, role, tenant)`                |
// | Unassign role from subject   | `RemoveGroupingPolicy(subject, role, tenant)`             |
// | Check subject tenant member  | `GetFilteredGroupingPolicy(0, subject)`                   |
// | Assign system role           | `AddNamedGroupingPolicy("g2", subject, role)`             |
// | Unassign system role         | `RemoveNamedGroupingPolicy("g2", subject, role)`          |
// | Query subject role in tenant | `GetFilteredGroupingPolicy(0, subject, role, tenant)`     |
// | Query role permissions       | `GetFilteredPolicy(0, tenant, role)`                      |
// | Query system role assignment | `HasNamedGroupingPolicy("g2", subject, role)`             |
// | Authorize request            | `Enforce(tenant, subject, obj, act)`                      |
//
// // Query subject role bindings in a tenant.
// RBAC.enforcer.GetFilteredGroupingPolicy(0, "user1", consts.AUTHZ_ROLE_ADMIN, DefaultTenant)
// // Query a subject's system-level role binding.
// RBAC.enforcer.HasNamedGroupingPolicy(systemRoleGrouping, consts.AUTHZ_USER_ROOT, consts.AUTHZ_SYSTEM_ROLE_ROOT)
// // Query permissions granted to a role in a tenant.
// RBAC.enforcer.GetFilteredPolicy(0, DefaultTenant, "admin")
// // Authorize a subject against a tenant-scoped permission.
// RBAC.enforcer.Enforce(DefaultTenant, "user1", "/api/authz/routes", "GET")

// AssignRole assigns subject to role inside tenant.
func (r *rbac) AssignRole(tenant string, subject string, role string) error {
	if subject == role {
		return nil
	}
	if _, err := r.enforcer.AddGroupingPolicy(subject, role, tenant); err != nil {
		return err
	}
	return nil
}

// UnassignRole removes a subject-role assignment from tenant.
func (r *rbac) UnassignRole(tenant string, subject string, role string) error {
	if _, err := r.enforcer.RemoveGroupingPolicy(subject, role, tenant); err != nil {
		return err
	}
	return nil
}

// SubjectInTenant reports whether subject has any role assignment inside tenant.
//
// Tenant membership is represented by Casbin grouping policies in the form
// subject, role, tenant. This check does not evaluate route permission; it only
// answers whether the subject belongs to the tenant authorization domain.
func (r *rbac) SubjectInTenant(tenant string, subject string) (bool, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return false, nil
	}
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = DefaultTenant
	}

	groupingPolicies, err := r.enforcer.GetFilteredGroupingPolicy(0, subject)
	if err != nil {
		return false, err
	}
	for _, policy := range groupingPolicies {
		if len(policy) >= 3 && strings.TrimSpace(policy[1]) != "" && policy[2] == tenant {
			return true, nil
		}
	}
	return false, nil
}

// SubjectsInTenant returns subjects with at least one role assignment inside tenant.
//
// It is used by IAM admin user list because IAM users do not store tenant_id.
// The tenant-visible user set is therefore derived from role bindings first and
// then joined back to user rows by subject ID.
func (r *rbac) SubjectsInTenant(tenant string) ([]string, error) {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = DefaultTenant
	}

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
func (r *rbac) AssignSystemRole(subject string, role string) error {
	if subject == role {
		return nil
	}
	if _, err := r.enforcer.AddNamedGroupingPolicy(systemRoleGrouping, subject, role); err != nil {
		return err
	}
	return nil
}

// UnassignSystemRole removes a subject's system-level role assignment.
func (r *rbac) UnassignSystemRole(subject string, role string) error {
	if _, err := r.enforcer.RemoveNamedGroupingPolicy(systemRoleGrouping, subject, role); err != nil {
		return err
	}
	return nil
}

// HasSystemRole reports whether subject explicitly holds a system-level role.
func (r *rbac) HasSystemRole(subject string, role string) (bool, error) {
	if subject == role {
		return false, nil
	}
	return r.enforcer.HasNamedGroupingPolicy(systemRoleGrouping, subject, role)
}

func isBuiltInSystemRole(subject string, role string) bool {
	return subject == consts.AUTHZ_USER_ROOT && role == consts.AUTHZ_SYSTEM_ROLE_ROOT
}
