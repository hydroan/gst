//nolint:gosec // G101: the sentences speak of passwords and credentials and hold none
package serviceiamaccount

import "github.com/hydroan/gst/i18n"

// This file holds the sentences the package answers a client with, and
// registers their translations.

// Signing in and out.
const (
	invalidUsernameOrPasswordMsg  = "invalid username or password"
	accountDisabledMsg            = "account disabled"
	accountLockedMsg              = "account locked"
	userNotMemberOfTenantMsg      = "user is not a member of tenant"
	authorizationUnavailableMsg   = "authorization unavailable"
	failedToCreateSessionIDMsg    = "failed to create session id"
	failedToTrackUserSessionMsg   = "failed to track user session"
	failedToVerifySecondFactorMsg = "failed to verify second factor"
	failedToLogOutMsg             = "failed to log out"
)

// The credentials a login, a signup or a new password must carry.
const (
	usernameRequiredMsg    = "username is required"
	passwordRequiredMsg    = "password is required"
	passwordTooShortMsg    = "password must be at least 6 characters long"
	passwordsDoNotMatchMsg = "passwords do not match"
)

// Signing up.
const (
	usernameAlreadyExistsMsg     = "username already exists"
	failedToCheckExistingUserMsg = "failed to check existing user"
	failedToCreateUserMsg        = "failed to create user"
)

// The user a request acts as or on.
const (
	sessionInvalidMsg            = "session invalid"
	currentUserNotFoundMsg       = "current user not found"
	userNotFoundMsg              = "user not found"
	userRequiredMsg              = "user is required"
	userIDRequiredMsg            = "user_id is required"
	failedToLoadCurrentUserMsg   = "failed to load current user"
	failedToLoadTargetUserMsg    = "failed to load target user"
	failedToLoadEmailIdentityMsg = "failed to load email identity"
)

// Changing and resetting a password.
const (
	changePasswordRequestRequiredMsg  = "change password request is required"
	resetPasswordRequestRequiredMsg   = "reset password request is required"
	oldPasswordRequiredMsg            = "old password is required"
	oldPasswordIncorrectMsg           = "old password is incorrect"
	newPasswordRequiredMsg            = "new password is required"
	passwordCredentialRequiredMsg     = "password credential is required"
	failedToQueryUserMsg              = "failed to query user"
	failedToLoadPasswordCredentialMsg = "failed to load password credential"
	failedToHashPasswordMsg           = "failed to hash password"
	failedToUpdatePasswordMsg         = "failed to update password"
	failedToRevokeOtherSessionsMsg    = "failed to revoke other sessions"
	failedToRevokeUserSessionsMsg     = "failed to revoke user sessions"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		invalidUsernameOrPasswordMsg:  "用户名或密码错误",
		accountDisabledMsg:            "账号已停用",
		accountLockedMsg:              "账号已锁定",
		userNotMemberOfTenantMsg:      "用户不属于该租户",
		authorizationUnavailableMsg:   "鉴权服务暂时不可用",
		failedToCreateSessionIDMsg:    "创建会话 ID 失败",
		failedToTrackUserSessionMsg:   "记录用户会话失败",
		failedToVerifySecondFactorMsg: "二次验证失败",
		failedToLogOutMsg:             "退出登录失败",

		usernameRequiredMsg:    "用户名不能为空",
		passwordRequiredMsg:    "密码不能为空",
		passwordTooShortMsg:    "密码长度不能少于 6 个字符",
		passwordsDoNotMatchMsg: "两次输入的密码不一致",

		usernameAlreadyExistsMsg:     "用户名已存在",
		failedToCheckExistingUserMsg: "检查用户是否已存在失败",
		failedToCreateUserMsg:        "创建用户失败",

		sessionInvalidMsg:            "会话无效",
		currentUserNotFoundMsg:       "当前用户不存在",
		userNotFoundMsg:              "用户不存在",
		userRequiredMsg:              "用户不能为空",
		userIDRequiredMsg:            "user_id为必填字段",
		failedToLoadCurrentUserMsg:   "加载当前用户失败",
		failedToLoadTargetUserMsg:    "加载目标用户失败",
		failedToLoadEmailIdentityMsg: "加载邮箱身份失败",

		changePasswordRequestRequiredMsg:  "缺少修改密码的请求内容",
		resetPasswordRequestRequiredMsg:   "缺少重置密码的请求内容",
		oldPasswordRequiredMsg:            "原密码不能为空",
		oldPasswordIncorrectMsg:           "原密码不正确",
		newPasswordRequiredMsg:            "新密码不能为空",
		passwordCredentialRequiredMsg:     "密码凭证不能为空",
		failedToQueryUserMsg:              "查询用户失败",
		failedToLoadPasswordCredentialMsg: "加载密码凭证失败",
		failedToHashPasswordMsg:           "密码加密失败",
		failedToUpdatePasswordMsg:         "更新密码失败",
		failedToRevokeOtherSessionsMsg:    "注销其他会话失败",
		failedToRevokeUserSessionsMsg:     "注销用户的会话失败",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		invalidUsernameOrPasswordMsg:  "使用者名稱或密碼錯誤",
		accountDisabledMsg:            "帳號已停用",
		accountLockedMsg:              "帳號已鎖定",
		userNotMemberOfTenantMsg:      "使用者不屬於這個租戶",
		authorizationUnavailableMsg:   "授權服務暫時無法使用",
		failedToCreateSessionIDMsg:    "建立工作階段 ID 失敗",
		failedToTrackUserSessionMsg:   "記錄使用者的工作階段失敗",
		failedToVerifySecondFactorMsg: "兩步驟驗證失敗",
		failedToLogOutMsg:             "登出失敗",

		usernameRequiredMsg:    "使用者名稱不可為空",
		passwordRequiredMsg:    "密碼不可為空",
		passwordTooShortMsg:    "密碼長度不能少於 6 個字元",
		passwordsDoNotMatchMsg: "兩次輸入的密碼不一致",

		usernameAlreadyExistsMsg:     "使用者名稱已存在",
		failedToCheckExistingUserMsg: "檢查使用者是否已存在失敗",
		failedToCreateUserMsg:        "建立使用者失敗",

		sessionInvalidMsg:            "工作階段無效",
		currentUserNotFoundMsg:       "目前的使用者不存在",
		userNotFoundMsg:              "使用者不存在",
		userRequiredMsg:              "使用者不可為空",
		userIDRequiredMsg:            "user_id為必填欄位",
		failedToLoadCurrentUserMsg:   "載入目前的使用者失敗",
		failedToLoadTargetUserMsg:    "載入目標使用者失敗",
		failedToLoadEmailIdentityMsg: "載入電子信箱身分失敗",

		changePasswordRequestRequiredMsg:  "缺少變更密碼的請求內容",
		resetPasswordRequestRequiredMsg:   "缺少重設密碼的請求內容",
		oldPasswordRequiredMsg:            "原密碼不可為空",
		oldPasswordIncorrectMsg:           "原密碼不正確",
		newPasswordRequiredMsg:            "新密碼不可為空",
		passwordCredentialRequiredMsg:     "密碼憑證不可為空",
		failedToQueryUserMsg:              "查詢使用者失敗",
		failedToLoadPasswordCredentialMsg: "載入密碼憑證失敗",
		failedToHashPasswordMsg:           "密碼加密失敗",
		failedToUpdatePasswordMsg:         "更新密碼失敗",
		failedToRevokeOtherSessionsMsg:    "撤銷其他工作階段失敗",
		failedToRevokeUserSessionsMsg:     "撤銷使用者的工作階段失敗",
	})
}
