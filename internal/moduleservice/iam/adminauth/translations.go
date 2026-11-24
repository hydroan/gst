package adminauth

import "github.com/hydroan/gst/i18n"

// The sentences the package answers a client with.
const (
	authorizationUnavailableMsg   = "authorization unavailable"
	permissionDeniedMsg           = "permission denied"
	failedToVerifyTargetTenantMsg = "failed to verify target tenant"
	targetUserOutsideTenantMsg    = "target user is outside tenant"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		authorizationUnavailableMsg:   "鉴权服务暂时不可用",
		permissionDeniedMsg:           "没有权限",
		failedToVerifyTargetTenantMsg: "校验目标租户失败",
		targetUserOutsideTenantMsg:    "目标用户不属于该租户",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		authorizationUnavailableMsg:   "授權服務暫時無法使用",
		permissionDeniedMsg:           "沒有權限",
		failedToVerifyTargetTenantMsg: "驗證目標租戶失敗",
		targetUserOutsideTenantMsg:    "目標使用者不屬於這個租戶",
	})
}
