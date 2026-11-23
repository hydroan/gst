package rbac

import "github.com/hydroan/gst/internal/i18n"

// The messages a refusal of Enforce is answered with: a subject that is
// anonymous or denied, 403, and a decision that could not be made, 500.
const (
	deniedMsg      = "permission denied"
	unavailableMsg = "authorization unavailable"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		deniedMsg:      "没有权限",
		unavailableMsg: "鉴权服务暂时不可用",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		deniedMsg:      "沒有權限",
		unavailableMsg: "授權服務暫時無法使用",
	})
}
