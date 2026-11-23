package authn

import "github.com/hydroan/gst/internal/i18n"

// SecondFactorRequiredMsg is the challenge a verifier reports with 401 when
// the account needs a second factor it has not submitted.
//
// It belongs to this package rather than to any one verifier because the
// challenge belongs to the login protocol, not to a kind of factor: it is the
// same sentence whether the installed verifier checks TOTP, SMS, or anything
// else. Proof-specific rejections stay the verifier's own wording.
const SecondFactorRequiredMsg = "second factor required"

// init registers the translations of the sentence above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		SecondFactorRequiredMsg: "需要二次验证",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		SecondFactorRequiredMsg: "需要兩步驟驗證",
	})
}
