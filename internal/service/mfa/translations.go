package servicemfa

import "github.com/hydroan/gst/i18n"

// This file holds the sentences the package answers a client with, and
// registers their translations.

// Who may act on the second factors of an account.
const (
	authenticationRequiredMsg               = "authentication required"
	freshAuthenticationRequiredMsg          = "fresh authentication required"
	failedToVerifyFreshAuthenticationMsg    = "failed to verify fresh authentication"
	userIDRequiredMsg                       = "user id is required"
	administrativeAuthorizationFailedMsg    = "administrative authorization failed"
	mfaAccountAdministratorNotConfiguredMsg = "MFA account administrator is not configured"
)

// Binding a TOTP device.
const (
	invalidOrExpiredTOTPBindingChallengeMsg = "invalid or expired TOTP binding challenge"
	failedToGenerateTOTPKeyMsg              = "failed to generate TOTP key"
	failedToGenerateQRCodeImageMsg          = "failed to generate QR code image"
	failedToIssueTOTPBindingChallengeMsg    = "failed to issue TOTP binding challenge"
	failedToLoadTOTPBindingChallengeMsg     = "failed to load TOTP binding challenge"
	failedToConsumeTOTPBindingChallengeMsg  = "failed to consume TOTP binding challenge"
	failedToConfirmTOTPBindingMsg           = "failed to confirm TOTP binding"
	failedToGenerateBackupCodesMsg          = "failed to generate backup codes"
	failedToHashBackupCodesMsg              = "failed to hash backup codes"
	failedToSaveDeviceMsg                   = "failed to save device"
)

// Verifying a second factor.
const (
	provideExactlyOneSecondFactorMsg     = "provide exactly one second factor"
	invalidTOTPCodeMsg                   = "invalid TOTP code"
	invalidBackupCodeMsg                 = "invalid backup code"
	tooManyFailedVerificationAttemptsMsg = "too many failed verification attempts"
	failedToVerifySecondFactorMsg        = "failed to verify second factor"
)

// Unbinding a device.
const (
	deviceIDRequiredMsg                    = "device_id is required"
	provideExactlyOneVerificationMethodMsg = "provide exactly one verification method"
	invalidVerificationMsg                 = "invalid verification"
	deviceNotFoundOrAlreadyUnboundMsg      = "device not found or already unbound"
	failedToListActiveTOTPDevicesMsg       = "failed to list active TOTP devices"
	failedToUnbindDeviceMsg                = "failed to unbind device"
	failedToBuildTOTPUnbindResponseMsg     = "failed to build TOTP unbind response"
)

// Reading and resetting the devices of an account.
const (
	failedToRetrieveDeviceInformationMsg = "failed to retrieve device information"
	failedToListTOTPDevicesMsg           = "failed to list TOTP devices"
	failedToResetTOTPDevicesMsg          = "failed to reset TOTP devices"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		authenticationRequiredMsg:               "请先登录",
		freshAuthenticationRequiredMsg:          "请重新登录后再操作",
		failedToVerifyFreshAuthenticationMsg:    "校验近期登录状态失败",
		userIDRequiredMsg:                       "用户 ID 不能为空",
		administrativeAuthorizationFailedMsg:    "管理员权限校验失败",
		mfaAccountAdministratorNotConfiguredMsg: "二次验证的账号管理员尚未配置",

		invalidOrExpiredTOTPBindingChallengeMsg: "TOTP 绑定凭据无效或已过期",
		failedToGenerateTOTPKeyMsg:              "生成 TOTP 密钥失败",
		failedToGenerateQRCodeImageMsg:          "生成二维码图片失败",
		failedToIssueTOTPBindingChallengeMsg:    "生成 TOTP 绑定凭据失败",
		failedToLoadTOTPBindingChallengeMsg:     "加载 TOTP 绑定凭据失败",
		failedToConsumeTOTPBindingChallengeMsg:  "核销 TOTP 绑定凭据失败",
		failedToConfirmTOTPBindingMsg:           "确认 TOTP 绑定失败",
		failedToGenerateBackupCodesMsg:          "生成备用码失败",
		failedToHashBackupCodesMsg:              "备用码加密失败",
		failedToSaveDeviceMsg:                   "保存设备失败",

		provideExactlyOneSecondFactorMsg:     "请只提供一种二次验证方式",
		invalidTOTPCodeMsg:                   "TOTP 验证码无效",
		invalidBackupCodeMsg:                 "备用码无效",
		tooManyFailedVerificationAttemptsMsg: "验证失败次数过多",
		failedToVerifySecondFactorMsg:        "二次验证失败",

		deviceIDRequiredMsg:                    "device_id为必填字段",
		provideExactlyOneVerificationMethodMsg: "请只提供一种验证方式",
		invalidVerificationMsg:                 "验证未通过",
		deviceNotFoundOrAlreadyUnboundMsg:      "设备不存在或已解绑",
		failedToListActiveTOTPDevicesMsg:       "查询已启用的 TOTP 设备失败",
		failedToUnbindDeviceMsg:                "解绑设备失败",
		failedToBuildTOTPUnbindResponseMsg:     "生成 TOTP 解绑结果失败",

		failedToRetrieveDeviceInformationMsg: "获取设备信息失败",
		failedToListTOTPDevicesMsg:           "查询 TOTP 设备失败",
		failedToResetTOTPDevicesMsg:          "重置 TOTP 设备失败",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		authenticationRequiredMsg:               "請先登入",
		freshAuthenticationRequiredMsg:          "請重新登入後再操作",
		failedToVerifyFreshAuthenticationMsg:    "驗證近期登入狀態失敗",
		userIDRequiredMsg:                       "使用者 ID 不可為空",
		administrativeAuthorizationFailedMsg:    "管理員權限驗證失敗",
		mfaAccountAdministratorNotConfiguredMsg: "兩步驟驗證的帳號管理員尚未設定",

		invalidOrExpiredTOTPBindingChallengeMsg: "TOTP 綁定憑據無效或已過期",
		failedToGenerateTOTPKeyMsg:              "產生 TOTP 金鑰失敗",
		failedToGenerateQRCodeImageMsg:          "產生 QR Code 圖片失敗",
		failedToIssueTOTPBindingChallengeMsg:    "產生 TOTP 綁定憑據失敗",
		failedToLoadTOTPBindingChallengeMsg:     "載入 TOTP 綁定憑據失敗",
		failedToConsumeTOTPBindingChallengeMsg:  "核銷 TOTP 綁定憑據失敗",
		failedToConfirmTOTPBindingMsg:           "確認 TOTP 綁定失敗",
		failedToGenerateBackupCodesMsg:          "產生備用碼失敗",
		failedToHashBackupCodesMsg:              "備用碼加密失敗",
		failedToSaveDeviceMsg:                   "儲存裝置失敗",

		provideExactlyOneSecondFactorMsg:     "請只提供一種兩步驟驗證方式",
		invalidTOTPCodeMsg:                   "TOTP 驗證碼無效",
		invalidBackupCodeMsg:                 "備用碼無效",
		tooManyFailedVerificationAttemptsMsg: "驗證失敗次數過多",
		failedToVerifySecondFactorMsg:        "兩步驟驗證失敗",

		deviceIDRequiredMsg:                    "device_id為必填欄位",
		provideExactlyOneVerificationMethodMsg: "請只提供一種驗證方式",
		invalidVerificationMsg:                 "驗證未通過",
		deviceNotFoundOrAlreadyUnboundMsg:      "裝置不存在或已解除綁定",
		failedToListActiveTOTPDevicesMsg:       "查詢已啟用的 TOTP 裝置失敗",
		failedToUnbindDeviceMsg:                "解除裝置綁定失敗",
		failedToBuildTOTPUnbindResponseMsg:     "產生 TOTP 解除綁定的結果失敗",

		failedToRetrieveDeviceInformationMsg: "取得裝置資訊失敗",
		failedToListTOTPDevicesMsg:           "查詢 TOTP 裝置失敗",
		failedToResetTOTPDevicesMsg:          "重設 TOTP 裝置失敗",
	})
}
