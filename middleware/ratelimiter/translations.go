package ratelimiter

import "github.com/hydroan/gst/internal/i18n"

// The messages the limiter refuses a request with: a client over its rate,
// 429, and a bucket that could not be read, 500.
const (
	tooManyRequestsMsg = "too many requests"
	unavailableMsg     = "rate limiter unavailable"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		tooManyRequestsMsg: "请求过于频繁，请稍后再试",
		unavailableMsg:     "限流服务暂时不可用",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		tooManyRequestsMsg: "請求過於頻繁，請稍後再試",
		unavailableMsg:     "流量限制服務暫時無法使用",
	})
}
