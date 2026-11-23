package jwt

import "github.com/hydroan/gst/internal/i18n"

// refusalMsg is the one message a refused token is answered with (see
// LogRefusal).
const refusalMsg = "invalid token"

// init registers the translations of the sentence above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		refusalMsg: "令牌无效",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		refusalMsg: "權杖無效",
	})
}
