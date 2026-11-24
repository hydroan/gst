package serviceauthz

import "github.com/hydroan/gst/i18n"

// The sentences the package answers a client with.
const (
	authorizationUnavailableMsg = "authorization unavailable"
	failedToLoadRoleBindingsMsg = "failed to load role bindings"
	failedToLoadRolesMsg        = "failed to load roles"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		authorizationUnavailableMsg: "鉴权服务暂时不可用",
		failedToLoadRoleBindingsMsg: "加载角色绑定失败",
		failedToLoadRolesMsg:        "加载角色失败",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		authorizationUnavailableMsg: "授權服務暫時無法使用",
		failedToLoadRoleBindingsMsg: "載入角色綁定失敗",
		failedToLoadRolesMsg:        "載入角色失敗",
	})
}
