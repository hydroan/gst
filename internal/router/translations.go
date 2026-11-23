package router

import "github.com/hydroan/gst/internal/i18n"

// The messages a request no handler serves is answered with: a path no
// route matches, 404, and a path asked for with a method it does not take,
// 405.
const (
	notFoundMsg         = "not found"
	methodNotAllowedMsg = "method not allowed"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		notFoundMsg:         "未找到",
		methodNotAllowedMsg: "不支持该请求方法",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		notFoundMsg:         "找不到",
		methodNotAllowedMsg: "不支援這個請求方法",
	})
}
