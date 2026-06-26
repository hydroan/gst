package rbac

import (
	"github.com/casbin/casbin/v3"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

var (
	Enforcer *casbin.SyncedEnforcer
	Adapter  *gormadapter.Adapter
)

type rbac struct {
	enforcer *casbin.SyncedEnforcer
	addapter *gormadapter.Adapter
}

// noop implements a no-op RBAC that safely does nothing.
// It is used when RBAC is disabled or the Casbin enforcer
// has not been initialized yet to avoid nil pointer panics.
type noop struct{}

func (noop) AddRole(name string) error                                          { return nil }
func (noop) RemoveRole(name string) error                                       { return nil }
func (noop) GrantPermission(role string, resource string, action string) error  { return nil }
func (noop) RevokePermission(role string, resource string, action string) error { return nil }
func (noop) AssignRole(subject string, role string) error                       { return nil }
func (noop) UnassignRole(subject string, role string) error                     { return nil }

func RBAC() types.RBAC {
	// When RBAC is disabled or Enforcer is not initialized,
	// return a safe no-op implementation to prevent panics.
	if Enforcer == nil {
		return noop{}
	}
	return &rbac{
		enforcer: Enforcer,
		addapter: Adapter,
	}
}

// AddRole is a no-op in Casbin, roles are created implicitly when used.
func (r *rbac) AddRole(name string) error {
	return nil
}

func (r *rbac) RemoveRole(name string) error {
	if _, err := r.enforcer.DeleteRole(name); err != nil {
		return err
	}
	return nil
}

func (r *rbac) GrantPermission(role string, resource string, action string) error {
	if _, err := r.enforcer.AddPermissionForUser(role, resource, action, "allow"); err != nil {
		return err
	}
	return nil
}

// RevokePermission removes policies for the given role with flexible behaviors:
// - resource=="" && action=="" : remove all policies for the role
// - resource=="" && action!="" : remove policies matching the role and action
// - resource!="" && action=="" : remove policies matching the role and resource
// - resource!="" && action!="" : remove the exact (role, resource, action, "allow") policy
func (r *rbac) RevokePermission(role string, resource string, action string) error {
	if len(resource) == 0 && len(action) == 0 {
		if _, err := r.enforcer.RemoveFilteredPolicy(0, role); err != nil {
			return err
		}
		return nil
	}
	if len(resource) == 0 && len(action) > 0 {
		if _, err := r.enforcer.RemoveFilteredPolicy(0, role, "", action); err != nil {
			return err
		}
		return nil
	}
	if len(action) == 0 && len(resource) > 0 {
		if _, err := r.enforcer.RemoveFilteredPolicy(0, role, resource); err != nil {
			return err
		}
		return nil
	}
	if _, err := r.enforcer.DeletePermissionForUser(role, resource, action, string(consts.EffectAllow)); err != nil {
		return err
	}
	return nil
}

func (r *rbac) AssignRole(subject string, role string) error {
	if _, err := r.enforcer.AddRoleForUser(subject, role); err != nil {
		return err
	}
	return nil
}

func (r *rbac) UnassignRole(subject string, role string) error {
	if _, err := r.enforcer.DeleteRoleForUser(subject, role); err != nil {
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
