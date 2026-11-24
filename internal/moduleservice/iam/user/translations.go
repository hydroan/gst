//nolint:gosec // G101: the sentences speak of passwords and credentials and hold none
package serviceiamuser

import "github.com/hydroan/gst/i18n"

// This file holds the sentences the package answers a client with, and
// registers their translations.

// Creating a user.
const (
	usernameRequiredMsg             = "username is required"
	usernameOrEmailAlreadyExistsMsg = "username or email already exists"
	failedToCreateUserMsg           = "failed to create user"
)

// Reading and changing a user.
const (
	userIDRequiredMsg           = "user id is required"
	usernameCannotBeEmptyMsg    = "username cannot be empty"
	usernameAlreadyExistsMsg    = "username already exists"
	noUpdatableFieldProvidedMsg = "no updatable field provided"
	invalidStatusMsg            = "invalid status: must be active, inactive, or locked"
	failedToUpdateUserMsg       = "failed to update user"
)

// Listing users.
const (
	failedToListUsersMsg  = "failed to list users"
	failedToCountUsersMsg = "failed to count users"
)

// The identities and credentials a user is shown with.
const (
	failedToLoadEmailIdentitiesMsg     = "failed to load email identities"
	failedToLoadPasswordCredentialsMsg = "failed to load password credentials"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		usernameRequiredMsg:             "用户名不能为空",
		usernameOrEmailAlreadyExistsMsg: "用户名或邮箱已存在",
		failedToCreateUserMsg:           "创建用户失败",

		userIDRequiredMsg:           "用户 ID 不能为空",
		usernameCannotBeEmptyMsg:    "用户名不能为空",
		usernameAlreadyExistsMsg:    "用户名已存在",
		noUpdatableFieldProvidedMsg: "没有提供可更新的字段",
		invalidStatusMsg:            "状态无效：只能是 active、inactive 或 locked",
		failedToUpdateUserMsg:       "更新用户失败",

		failedToListUsersMsg:  "查询用户列表失败",
		failedToCountUsersMsg: "统计用户数量失败",

		failedToLoadEmailIdentitiesMsg:     "加载邮箱身份失败",
		failedToLoadPasswordCredentialsMsg: "加载密码凭证失败",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		usernameRequiredMsg:             "使用者名稱不可為空",
		usernameOrEmailAlreadyExistsMsg: "使用者名稱或電子信箱已存在",
		failedToCreateUserMsg:           "建立使用者失敗",

		userIDRequiredMsg:           "使用者 ID 不可為空",
		usernameCannotBeEmptyMsg:    "使用者名稱不可為空",
		usernameAlreadyExistsMsg:    "使用者名稱已存在",
		noUpdatableFieldProvidedMsg: "沒有提供可更新的欄位",
		invalidStatusMsg:            "狀態無效：只能是 active、inactive 或 locked",
		failedToUpdateUserMsg:       "更新使用者失敗",

		failedToListUsersMsg:  "查詢使用者清單失敗",
		failedToCountUsersMsg: "統計使用者數量失敗",

		failedToLoadEmailIdentitiesMsg:     "載入電子信箱身分失敗",
		failedToLoadPasswordCredentialsMsg: "載入密碼憑證失敗",
	})
}
