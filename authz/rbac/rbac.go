package rbac

import (
	"github.com/casbin/casbin/v3"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// DefaultTenant is the built-in authorization domain used when no tenant
// resolver is configured by the application.
const DefaultTenant = "default"

var (
	Enforcer *casbin.SyncedEnforcer
	Adapter  *gormadapter.Adapter
)

type rbac struct {
	enforcer *casbin.SyncedEnforcer
	adapter  *gormadapter.Adapter
}

// noop implements a no-op RBAC that safely does nothing.
// It is used when RBAC is disabled or the Casbin enforcer
// has not been initialized yet to avoid nil pointer panics.
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
// | Query subject role in tenant | `GetFilteredGroupingPolicy(0, subject, role, tenant)`     |
// | Query role permissions       | `GetFilteredPolicy(0, tenant, role)`                      |
// | Authorize request            | `Enforce(tenant, subject, obj, act)`                      |
//
// // Query subject role bindings in a tenant.
// RBAC.enforcer.GetFilteredGroupingPolicy(0, "root", "admin", DefaultTenant)
// // Query permissions granted to a role in a tenant.
// RBAC.enforcer.GetFilteredPolicy(0, DefaultTenant, "admin")
// // Authorize a subject against a tenant-scoped permission.
// RBAC.enforcer.Enforce(DefaultTenant, "root", "/api/authz/routes", "GET")

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
