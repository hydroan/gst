//nolint:gosec // G101: the sentences speak of passwords and credentials and hold none
package serviceemail

import "github.com/hydroan/gst/i18n"

// This file holds the sentences the package answers a client with, and
// registers their translations.

// The account gateway the flows read accounts through.
const (
	emailAccountGatewayNotConfiguredMsg          = "email account gateway is not configured"
	emailAccountGatewayReturnedInvalidAccountMsg = "email account gateway returned an invalid account"
)

// The address a flow mails.
const (
	emailMustBeValidEmailAddressMsg = "email must be a valid email address"
)

// Changing the email of an account: what the request is refused for.
const (
	authenticationRequiredMsg                   = "authentication required"
	currentAccountRequiredMsg                   = "current account is required"
	currentAccountIDRequiredMsg                 = "current account id is required"
	currentAccountNotActiveMsg                  = "current account is not active"
	currentEmailRequiredMsg                     = "current email is required"
	currentPasswordIncorrectMsg                 = "current password is incorrect"
	newEmailRequiredMsg                         = "new email is required"
	newEmailMustBeValidEmailAddressMsg          = "new email must be a valid email address"
	newEmailMustBeDifferentFromCurrentEmailMsg  = "new email must be different from current email"
	newEmailAlreadyInUseMsg                     = "new email is already in use"
	emailChangeAccountIDRequiredMsg             = "email change account id is required"
	emailChangeOldEmailRequiredMsg              = "email change old email is required"
	emailChangeNewEmailRequiredMsg              = "email change new email is required"
	emailChangeOldAndNewEmailMustBeDifferentMsg = "email change old and new email must be different"
)

// Changing the email of an account: what fails on the server.
const (
	emailChangeConfirmationThrottledMsg               = "email change confirmation throttled"
	emailChangeCancellationThrottledMsg               = "email change cancellation throttled"
	failedToLoadCurrentAccountMsg                     = "failed to load current account"
	failedToVerifyCurrentPasswordMsg                  = "failed to verify current password"
	failedToLookUpTargetEmailMsg                      = "failed to look up target email"
	failedToUpdateEmailChangeStateMsg                 = "failed to update email change state"
	failedToMarkEmailChangeAsCanceledMsg              = "failed to mark email change as canceled"
	failedToCheckEmailChangeCancellationStateMsg      = "failed to check email change cancellation state"
	failedToClearEmailChangeCancellationMarkerMsg     = "failed to clear email change cancellation marker"
	failedToReserveEmailChangeConfirmationThrottleMsg = "failed to reserve email change confirmation throttle"
	failedToReserveEmailChangeCancellationThrottleMsg = "failed to reserve email change cancellation throttle"
	failedToReserveEmailChangeResendThrottleMsg       = "failed to reserve email change resend throttle"
	failedToIssueEmailChangeConfirmationFlowMsg       = "failed to issue email change confirmation flow"
	failedToIssueEmailChangeCancellationFlowMsg       = "failed to issue email change cancellation flow"
	failedToIssueEmailChangeResendFlowMsg             = "failed to issue email change resend flow"
	failedToDispatchEmailChangeConfirmationMsg        = "failed to dispatch email change confirmation"
	failedToDispatchEmailChangeCancellationMsg        = "failed to dispatch email change cancellation"
	failedToDispatchEmailChangeResendConfirmationMsg  = "failed to dispatch email change resend confirmation"
	failedToConsumeEmailChangeConfirmationFlowMsg     = "failed to consume email change confirmation flow"
	failedToConsumeEmailChangeCancellationFlowMsg     = "failed to consume email change cancellation flow"
	failedToLoadEmailChangeConfirmationAccountMsg     = "failed to load email change confirmation account"
	failedToLoadEmailChangeCancellationAccountMsg     = "failed to load email change cancellation account"
)

// Resetting a password.
const (
	passwordResetAccountIDRequiredMsg       = "password reset account id is required"
	failedToLoadPasswordResetAccountMsg     = "failed to load password reset account"
	failedToReservePasswordResetThrottleMsg = "failed to reserve password reset throttle"
	failedToIssuePasswordResetFlowMsg       = "failed to issue password reset flow"
	failedToDispatchPasswordResetEmailMsg   = "failed to dispatch password reset email"
	failedToConsumePasswordResetFlowMsg     = "failed to consume password reset flow"
	failedToUpdatePasswordMsg               = "failed to update password"
)

// Verifying the email of an account.
const (
	verificationAccountIDRequiredMsg              = "verification account id is required"
	failedToLoadVerificationAccountMsg            = "failed to load verification account"
	failedToLoadVerificationResendAccountMsg      = "failed to load verification resend account"
	failedToReserveVerificationRequestThrottleMsg = "failed to reserve verification request throttle"
	failedToReserveVerificationResendThrottleMsg  = "failed to reserve verification resend throttle"
	failedToIssueVerificationFlowMsg              = "failed to issue verification flow"
	failedToIssueVerificationResendFlowMsg        = "failed to issue verification resend flow"
	failedToDispatchVerificationEmailMsg          = "failed to dispatch verification email"
	failedToDispatchVerificationResendEmailMsg    = "failed to dispatch verification resend email"
	failedToConsumeVerificationFlowMsg            = "failed to consume verification flow"
	failedToUpdateEmailVerificationStateMsg       = "failed to update email verification state"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		emailAccountGatewayNotConfiguredMsg:          "邮箱账号网关尚未配置",
		emailAccountGatewayReturnedInvalidAccountMsg: "邮箱账号网关返回了无效的账号",

		emailMustBeValidEmailAddressMsg: "邮箱格式不正确",

		authenticationRequiredMsg:                   "请先登录",
		currentAccountRequiredMsg:                   "当前账号不能为空",
		currentAccountIDRequiredMsg:                 "当前账号 ID 不能为空",
		currentAccountNotActiveMsg:                  "当前账号未启用",
		currentEmailRequiredMsg:                     "当前邮箱不能为空",
		currentPasswordIncorrectMsg:                 "当前密码不正确",
		newEmailRequiredMsg:                         "新邮箱不能为空",
		newEmailMustBeValidEmailAddressMsg:          "新邮箱格式不正确",
		newEmailMustBeDifferentFromCurrentEmailMsg:  "新邮箱不能与当前邮箱相同",
		newEmailAlreadyInUseMsg:                     "新邮箱已被使用",
		emailChangeAccountIDRequiredMsg:             "邮箱变更的账号 ID 不能为空",
		emailChangeOldEmailRequiredMsg:              "邮箱变更的原邮箱不能为空",
		emailChangeNewEmailRequiredMsg:              "邮箱变更的新邮箱不能为空",
		emailChangeOldAndNewEmailMustBeDifferentMsg: "邮箱变更的新旧邮箱不能相同",

		emailChangeConfirmationThrottledMsg:               "确认邮箱变更的操作过于频繁，请稍后再试",
		emailChangeCancellationThrottledMsg:               "取消邮箱变更的操作过于频繁，请稍后再试",
		failedToLoadCurrentAccountMsg:                     "加载当前账号失败",
		failedToVerifyCurrentPasswordMsg:                  "校验当前密码失败",
		failedToLookUpTargetEmailMsg:                      "查询目标邮箱失败",
		failedToUpdateEmailChangeStateMsg:                 "更新邮箱变更状态失败",
		failedToMarkEmailChangeAsCanceledMsg:              "标记邮箱变更为已取消失败",
		failedToCheckEmailChangeCancellationStateMsg:      "检查邮箱变更的取消状态失败",
		failedToClearEmailChangeCancellationMarkerMsg:     "清除邮箱变更取消标记失败",
		failedToReserveEmailChangeConfirmationThrottleMsg: "登记确认邮箱变更的限频失败",
		failedToReserveEmailChangeCancellationThrottleMsg: "登记取消邮箱变更的限频失败",
		failedToReserveEmailChangeResendThrottleMsg:       "登记重新发送邮箱变更确认的限频失败",
		failedToIssueEmailChangeConfirmationFlowMsg:       "生成确认邮箱变更的凭据失败",
		failedToIssueEmailChangeCancellationFlowMsg:       "生成取消邮箱变更的凭据失败",
		failedToIssueEmailChangeResendFlowMsg:             "生成重新发送邮箱变更确认的凭据失败",
		failedToDispatchEmailChangeConfirmationMsg:        "发送确认邮箱变更的邮件失败",
		failedToDispatchEmailChangeCancellationMsg:        "发送取消邮箱变更的邮件失败",
		failedToDispatchEmailChangeResendConfirmationMsg:  "重新发送确认邮箱变更的邮件失败",
		failedToConsumeEmailChangeConfirmationFlowMsg:     "核销确认邮箱变更的凭据失败",
		failedToConsumeEmailChangeCancellationFlowMsg:     "核销取消邮箱变更的凭据失败",
		failedToLoadEmailChangeConfirmationAccountMsg:     "加载确认邮箱变更的账号失败",
		failedToLoadEmailChangeCancellationAccountMsg:     "加载取消邮箱变更的账号失败",

		passwordResetAccountIDRequiredMsg:       "重置密码的账号 ID 不能为空",
		failedToLoadPasswordResetAccountMsg:     "加载重置密码的账号失败",
		failedToReservePasswordResetThrottleMsg: "登记重置密码的限频失败",
		failedToIssuePasswordResetFlowMsg:       "生成重置密码的凭据失败",
		failedToDispatchPasswordResetEmailMsg:   "发送重置密码的邮件失败",
		failedToConsumePasswordResetFlowMsg:     "核销重置密码的凭据失败",
		failedToUpdatePasswordMsg:               "更新密码失败",

		verificationAccountIDRequiredMsg:              "邮箱验证的账号 ID 不能为空",
		failedToLoadVerificationAccountMsg:            "加载待验证邮箱的账号失败",
		failedToLoadVerificationResendAccountMsg:      "加载重新发送验证邮件的账号失败",
		failedToReserveVerificationRequestThrottleMsg: "登记邮箱验证请求的限频失败",
		failedToReserveVerificationResendThrottleMsg:  "登记重新发送邮箱验证的限频失败",
		failedToIssueVerificationFlowMsg:              "生成邮箱验证的凭据失败",
		failedToIssueVerificationResendFlowMsg:        "生成重新发送邮箱验证的凭据失败",
		failedToDispatchVerificationEmailMsg:          "发送验证邮件失败",
		failedToDispatchVerificationResendEmailMsg:    "重新发送验证邮件失败",
		failedToConsumeVerificationFlowMsg:            "核销邮箱验证的凭据失败",
		failedToUpdateEmailVerificationStateMsg:       "更新邮箱验证状态失败",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		emailAccountGatewayNotConfiguredMsg:          "電子信箱帳號閘道尚未設定",
		emailAccountGatewayReturnedInvalidAccountMsg: "電子信箱帳號閘道傳回了無效的帳號",

		emailMustBeValidEmailAddressMsg: "電子信箱格式不正確",

		authenticationRequiredMsg:                   "請先登入",
		currentAccountRequiredMsg:                   "目前的帳號不可為空",
		currentAccountIDRequiredMsg:                 "目前的帳號 ID 不可為空",
		currentAccountNotActiveMsg:                  "目前的帳號尚未啟用",
		currentEmailRequiredMsg:                     "目前的電子信箱不可為空",
		currentPasswordIncorrectMsg:                 "目前的密碼不正確",
		newEmailRequiredMsg:                         "新的電子信箱不可為空",
		newEmailMustBeValidEmailAddressMsg:          "新的電子信箱格式不正確",
		newEmailMustBeDifferentFromCurrentEmailMsg:  "新的電子信箱不能與目前的相同",
		newEmailAlreadyInUseMsg:                     "新的電子信箱已被使用",
		emailChangeAccountIDRequiredMsg:             "變更電子信箱的帳號 ID 不可為空",
		emailChangeOldEmailRequiredMsg:              "變更電子信箱的原電子信箱不可為空",
		emailChangeNewEmailRequiredMsg:              "變更電子信箱的新電子信箱不可為空",
		emailChangeOldAndNewEmailMustBeDifferentMsg: "變更電子信箱時，新舊電子信箱不能相同",

		emailChangeConfirmationThrottledMsg:               "確認變更電子信箱的操作過於頻繁，請稍後再試",
		emailChangeCancellationThrottledMsg:               "取消變更電子信箱的操作過於頻繁，請稍後再試",
		failedToLoadCurrentAccountMsg:                     "載入目前的帳號失敗",
		failedToVerifyCurrentPasswordMsg:                  "驗證目前的密碼失敗",
		failedToLookUpTargetEmailMsg:                      "查詢目標電子信箱失敗",
		failedToUpdateEmailChangeStateMsg:                 "更新電子信箱變更狀態失敗",
		failedToMarkEmailChangeAsCanceledMsg:              "將電子信箱變更標記為已取消失敗",
		failedToCheckEmailChangeCancellationStateMsg:      "檢查電子信箱變更的取消狀態失敗",
		failedToClearEmailChangeCancellationMarkerMsg:     "清除電子信箱變更的取消標記失敗",
		failedToReserveEmailChangeConfirmationThrottleMsg: "登記確認變更電子信箱的頻率限制失敗",
		failedToReserveEmailChangeCancellationThrottleMsg: "登記取消變更電子信箱的頻率限制失敗",
		failedToReserveEmailChangeResendThrottleMsg:       "登記重新寄送電子信箱變更確認信的頻率限制失敗",
		failedToIssueEmailChangeConfirmationFlowMsg:       "產生確認變更電子信箱的憑據失敗",
		failedToIssueEmailChangeCancellationFlowMsg:       "產生取消變更電子信箱的憑據失敗",
		failedToIssueEmailChangeResendFlowMsg:             "產生重新寄送電子信箱變更確認信的憑據失敗",
		failedToDispatchEmailChangeConfirmationMsg:        "寄送確認變更電子信箱的信件失敗",
		failedToDispatchEmailChangeCancellationMsg:        "寄送取消變更電子信箱的信件失敗",
		failedToDispatchEmailChangeResendConfirmationMsg:  "重新寄送確認變更電子信箱的信件失敗",
		failedToConsumeEmailChangeConfirmationFlowMsg:     "核銷確認變更電子信箱的憑據失敗",
		failedToConsumeEmailChangeCancellationFlowMsg:     "核銷取消變更電子信箱的憑據失敗",
		failedToLoadEmailChangeConfirmationAccountMsg:     "載入確認變更電子信箱的帳號失敗",
		failedToLoadEmailChangeCancellationAccountMsg:     "載入取消變更電子信箱的帳號失敗",

		passwordResetAccountIDRequiredMsg:       "重設密碼的帳號 ID 不可為空",
		failedToLoadPasswordResetAccountMsg:     "載入重設密碼的帳號失敗",
		failedToReservePasswordResetThrottleMsg: "登記重設密碼的頻率限制失敗",
		failedToIssuePasswordResetFlowMsg:       "產生重設密碼的憑據失敗",
		failedToDispatchPasswordResetEmailMsg:   "寄送重設密碼的信件失敗",
		failedToConsumePasswordResetFlowMsg:     "核銷重設密碼的憑據失敗",
		failedToUpdatePasswordMsg:               "更新密碼失敗",

		verificationAccountIDRequiredMsg:              "電子信箱驗證的帳號 ID 不可為空",
		failedToLoadVerificationAccountMsg:            "載入待驗證電子信箱的帳號失敗",
		failedToLoadVerificationResendAccountMsg:      "載入重新寄送驗證信的帳號失敗",
		failedToReserveVerificationRequestThrottleMsg: "登記電子信箱驗證請求的頻率限制失敗",
		failedToReserveVerificationResendThrottleMsg:  "登記重新寄送電子信箱驗證的頻率限制失敗",
		failedToIssueVerificationFlowMsg:              "產生電子信箱驗證的憑據失敗",
		failedToIssueVerificationResendFlowMsg:        "產生重新寄送電子信箱驗證的憑據失敗",
		failedToDispatchVerificationEmailMsg:          "寄送驗證信失敗",
		failedToDispatchVerificationResendEmailMsg:    "重新寄送驗證信失敗",
		failedToConsumeVerificationFlowMsg:            "核銷電子信箱驗證的憑據失敗",
		failedToUpdateEmailVerificationStateMsg:       "更新電子信箱驗證狀態失敗",
	})
}
