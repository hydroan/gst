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

func (noop) AddRole(tenant string, role string) error    { return nil }
func (noop) RemoveRole(tenant string, role string) error { return nil }
func (noop) GrantPermission(tenant string, role string, object string, action string) error {
	return nil
}

func (noop) RevokePermission(tenant string, role string, object string, action string) error {
	return nil
}
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

// AddRole is a no-op because Casbin creates roles implicitly when a tenant
// receives permissions or grouping policies for that role.
func (r *rbac) AddRole(tenant string, role string) error {
	return nil
}

// RemoveRole removes all policies and subject assignments for role in tenant.
func (r *rbac) RemoveRole(tenant string, role string) error {
	policyErr := r.RevokePermission(tenant, role, "", "")
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

// RevokePermission removes policies for the given role with flexible behaviors:
// - object=="" && action=="" : remove all policies for role in tenant
// - object=="" && action!="" : remove policies matching tenant, role, and action
// - object!="" && action=="" : remove policies matching tenant, role, and object
// - object!="" && action!="" : remove the exact tenant, role, object, action policy
func (r *rbac) RevokePermission(tenant string, role string, object string, action string) error {
	if len(object) == 0 && len(action) == 0 {
		if _, err := r.enforcer.RemoveFilteredPolicy(0, tenant, role); err != nil {
			return err
		}
		return nil
	}
	if len(object) == 0 && len(action) > 0 {
		if _, err := r.enforcer.RemoveFilteredPolicy(0, tenant, role, "", action); err != nil {
			return err
		}
		return nil
	}
	if len(action) == 0 && len(object) > 0 {
		if _, err := r.enforcer.RemoveFilteredPolicy(0, tenant, role, object); err != nil {
			return err
		}
		return nil
	}
	if _, err := r.enforcer.RemovePolicy(tenant, role, object, action, string(consts.EffectAllow)); err != nil {
		return err
	}
	return nil
}

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

// | 操作             | 函数                                  |
// | ---------------- | ------------------------------------- |
// | 添加角色权限     | `AddPolicy(role, obj, act)`           |
// | 删除角色权限     | `RemovePolicy(...)`                   |
// | 给用户授权角色   | `AddGroupingPolicy(user, role)`       |
// | 删除用户授权     | `RemoveGroupingPolicy(user, role)`    |
// | 查询用户角色     | `GetRolesForUser(user)`               |
// | 查询角色权限     | `GetPermissionsForUser(role)`         |
// | 查询用户所有权限 | `GetImplicitPermissionsForUser(user)` |

// // 查询用户拥有的角色
// RBAC.enforcer.GetRolesForUser("root")
// // 查询角色拥有的权限
// RBAC.enforcer.GetFilteredPolicy(0, "admin")
// // 查询用户拥有的权限（继承）
// RBAC.enforcer.GetImplicitPermissionsForUser("root")
