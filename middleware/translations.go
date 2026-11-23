package middleware

import "github.com/hydroan/gst/internal/i18n"

// This file holds the sentences the middlewares of the package answer a
// client with, and registers their translations.

// The messages the IP filters refuse a request with, 403 each: a client
// address that cannot be read, and one the lists do not admit.
const (
	invalidClientIPMsg = "invalid client IP"
	accessDeniedMsg    = "access denied"
)

// bodyTooLargeMsg answers, with 400, a request whose body is over the limit.
const bodyTooLargeMsg = "request body too large"

// timeoutMsg answers, with 504, a request whose response had not started by
// its deadline.
const timeoutMsg = "request timeout"

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		invalidClientIPMsg: "客户端 IP 无效",
		accessDeniedMsg:    "拒绝访问",

		bodyTooLargeMsg: "请求体过大",
		timeoutMsg:      "请求超时",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		invalidClientIPMsg: "用戶端 IP 無效",
		accessDeniedMsg:    "拒絕存取",

		bodyTooLargeMsg: "請求主體過大",
		timeoutMsg:      "請求逾時",
	})
}
