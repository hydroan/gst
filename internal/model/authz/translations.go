package modelauthz

import "github.com/hydroan/gst/i18n"

// This file holds the sentences the package answers a client with, and
// registers their translations.

// Writing a menu.
const (
	authorizationUnavailableMsg    = "authorization unavailable"
	onlySystemAdminsModifyMenusMsg = "only system administrators may modify menus"
)

// Writing a role.
const (
	roleNameRequiredMsg            = "role name is required"
	roleIDRequiredMsg              = "role id is required"
	systemRootReservedMsg          = "system_root is reserved for the system role"
	adminReservedMsg               = "admin is reserved for the built-in tenant administrator role"
	authenticatedReservedMsg       = "authenticated is reserved for the implicit role of every authenticated subject"
	failedToRemoveRolePoliciesMsg  = "failed to remove the role's policies"
	failedToSyncRolePermissionsMsg = "failed to sync the role's permissions"
)

// Binding a role to a subject.
const (
	subjectIDRequiredMsg                   = "subject_id is required"
	bindingRoleIDRequiredMsg               = "role_id is required"
	roleTenantDoesNotMatchBindingTenantMsg = "role tenant does not match binding tenant"
	failedToAssignRoleMsg                  = "failed to assign the role"
	failedToUnassignRoleMsg                = "failed to unassign the role"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		authorizationUnavailableMsg:    "鉴权服务暂时不可用",
		onlySystemAdminsModifyMenusMsg: "只有系统管理员可以修改菜单",

		roleNameRequiredMsg:            "角色名称不能为空",
		roleIDRequiredMsg:              "角色 ID 不能为空",
		systemRootReservedMsg:          "system_root 是系统角色的保留名",
		adminReservedMsg:               "admin 是内置租户管理员角色的保留名",
		authenticatedReservedMsg:       "authenticated 是所有已登录主体隐含角色的保留名",
		failedToRemoveRolePoliciesMsg:  "移除角色的策略失败",
		failedToSyncRolePermissionsMsg: "同步角色的权限失败",

		subjectIDRequiredMsg:                   "subject_id为必填字段",
		bindingRoleIDRequiredMsg:               "role_id为必填字段",
		roleTenantDoesNotMatchBindingTenantMsg: "角色所属租户与绑定的租户不一致",
		failedToAssignRoleMsg:                  "分配角色失败",
		failedToUnassignRoleMsg:                "取消角色分配失败",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		authorizationUnavailableMsg:    "授權服務暫時無法使用",
		onlySystemAdminsModifyMenusMsg: "只有系統管理員可以修改選單",

		roleNameRequiredMsg:            "角色名稱不可為空",
		roleIDRequiredMsg:              "角色 ID 不可為空",
		systemRootReservedMsg:          "system_root 是系統角色的保留名稱",
		adminReservedMsg:               "admin 是內建租戶管理員角色的保留名稱",
		authenticatedReservedMsg:       "authenticated 是所有已登入主體隱含角色的保留名稱",
		failedToRemoveRolePoliciesMsg:  "移除角色的政策失敗",
		failedToSyncRolePermissionsMsg: "同步角色的權限失敗",

		subjectIDRequiredMsg:                   "subject_id為必填欄位",
		bindingRoleIDRequiredMsg:               "role_id為必填欄位",
		roleTenantDoesNotMatchBindingTenantMsg: "角色所屬的租戶與綁定的租戶不一致",
		failedToAssignRoleMsg:                  "指派角色失敗",
		failedToUnassignRoleMsg:                "取消角色指派失敗",
	})
}
