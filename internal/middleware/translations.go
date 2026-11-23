package middleware

import "github.com/hydroan/gst/internal/i18n"

// The sentences the built-in chain answers a client with.
const (
	// unavailableMsg answers, with 503, a request the breaker refused.
	unavailableMsg = "service unavailable"
	// malformedQueryMsg answers, with 400, a query string that fails to
	// parse.
	malformedQueryMsg = "malformed query string"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		unavailableMsg:    "服务暂时不可用",
		malformedQueryMsg: "查询字符串格式有误",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		unavailableMsg:    "服務暫時無法使用",
		malformedQueryMsg: "查詢字串格式有誤",
	})
}
