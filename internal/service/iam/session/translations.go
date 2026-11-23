//nolint:gosec // G101: the sentences speak of passwords and credentials and hold none
package serviceiamsession

import "github.com/hydroan/gst/i18n"

// This file holds the sentences the package answers a client with, and
// registers their translations.

// The session a request presents.
const (
	noSessionMsg              = "no session"
	sessionInvalidMsg         = "session invalid"
	passwordChangeRequiredMsg = "password change required before using this resource"
	accountDisabledMsg        = "account disabled"
	accountLockedMsg          = "account locked"
)

// Reading and ending sessions.
const (
	sessionIDRequiredMsg                  = "session id is required"
	sessionNotFoundMsg                    = "session not found"
	forbiddenMsg                          = "forbidden"
	authorizationUnavailableMsg           = "authorization unavailable"
	userIDRequiredMsg                     = "user id is required"
	userNotFoundMsg                       = "user not found"
	onlineWithinMustBePositiveDurationMsg = "online_within must be a positive duration"
)

// The session store.
const (
	sessionTTLNegativeMsg             = "session ttl is negative"
	failedToStoreSessionMsg           = "failed to store session"
	failedToRemoveSessionIndexMsg     = "failed to remove session index"
	failedToTouchSessionMsg           = "failed to touch session"
	failedToDeleteSessionMsg          = "failed to delete session"
	failedToDeleteUserSessionIndexMsg = "failed to delete user session index"
	failedToListSessionsMsg           = "failed to list sessions"
	failedToListUserSessionsMsg       = "failed to list user sessions"
	failedToListOnlineSessionsMsg     = "failed to list online sessions"
	failedToPruneExpiredSessionsMsg   = "failed to prune expired sessions"
)

// Loading what a session refers to.
const (
	failedToLoadSessionMsg             = "failed to load session"
	failedToLoadTargetSessionMsg       = "failed to load target session"
	failedToLoadSessionOwnerMsg        = "failed to load session owner"
	failedToLoadUserMsg                = "failed to load user"
	failedToLoadEmailIdentityMsg       = "failed to load email identity"
	failedToLoadPasswordCredentialMsg  = "failed to load password credential"
	failedToRefreshSessionUserStateMsg = "failed to refresh session user state"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		noSessionMsg:              "未登录",
		sessionInvalidMsg:         "会话无效",
		passwordChangeRequiredMsg: "请先修改密码后再访问",
		accountDisabledMsg:        "账号已停用",
		accountLockedMsg:          "账号已锁定",

		sessionIDRequiredMsg:                  "会话 ID 不能为空",
		sessionNotFoundMsg:                    "会话不存在",
		forbiddenMsg:                          "禁止访问",
		authorizationUnavailableMsg:           "鉴权服务暂时不可用",
		userIDRequiredMsg:                     "用户 ID 不能为空",
		userNotFoundMsg:                       "用户不存在",
		onlineWithinMustBePositiveDurationMsg: "online_within 必须是正的时长",

		sessionTTLNegativeMsg:             "会话有效期不能为负数",
		failedToStoreSessionMsg:           "保存会话失败",
		failedToRemoveSessionIndexMsg:     "移除会话索引失败",
		failedToTouchSessionMsg:           "更新会话活跃时间失败",
		failedToDeleteSessionMsg:          "删除会话失败",
		failedToDeleteUserSessionIndexMsg: "删除用户会话索引失败",
		failedToListSessionsMsg:           "查询会话列表失败",
		failedToListUserSessionsMsg:       "查询用户的会话失败",
		failedToListOnlineSessionsMsg:     "查询在线会话失败",
		failedToPruneExpiredSessionsMsg:   "清理过期会话失败",

		failedToLoadSessionMsg:             "加载会话失败",
		failedToLoadTargetSessionMsg:       "加载目标会话失败",
		failedToLoadSessionOwnerMsg:        "加载会话所属用户失败",
		failedToLoadUserMsg:                "加载用户失败",
		failedToLoadEmailIdentityMsg:       "加载邮箱身份失败",
		failedToLoadPasswordCredentialMsg:  "加载密码凭证失败",
		failedToRefreshSessionUserStateMsg: "刷新会话的用户状态失败",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		noSessionMsg:              "尚未登入",
		sessionInvalidMsg:         "工作階段無效",
		passwordChangeRequiredMsg: "請先變更密碼後再存取",
		accountDisabledMsg:        "帳號已停用",
		accountLockedMsg:          "帳號已鎖定",

		sessionIDRequiredMsg:                  "工作階段 ID 不可為空",
		sessionNotFoundMsg:                    "工作階段不存在",
		forbiddenMsg:                          "禁止存取",
		authorizationUnavailableMsg:           "授權服務暫時無法使用",
		userIDRequiredMsg:                     "使用者 ID 不可為空",
		userNotFoundMsg:                       "使用者不存在",
		onlineWithinMustBePositiveDurationMsg: "online_within 必須是正的時間長度",

		sessionTTLNegativeMsg:             "工作階段的有效期限不能是負數",
		failedToStoreSessionMsg:           "儲存工作階段失敗",
		failedToRemoveSessionIndexMsg:     "移除工作階段索引失敗",
		failedToTouchSessionMsg:           "更新工作階段的活動時間失敗",
		failedToDeleteSessionMsg:          "刪除工作階段失敗",
		failedToDeleteUserSessionIndexMsg: "刪除使用者的工作階段索引失敗",
		failedToListSessionsMsg:           "查詢工作階段清單失敗",
		failedToListUserSessionsMsg:       "查詢使用者的工作階段失敗",
		failedToListOnlineSessionsMsg:     "查詢線上的工作階段失敗",
		failedToPruneExpiredSessionsMsg:   "清理過期的工作階段失敗",

		failedToLoadSessionMsg:             "載入工作階段失敗",
		failedToLoadTargetSessionMsg:       "載入目標工作階段失敗",
		failedToLoadSessionOwnerMsg:        "載入工作階段所屬的使用者失敗",
		failedToLoadUserMsg:                "載入使用者失敗",
		failedToLoadEmailIdentityMsg:       "載入電子信箱身分失敗",
		failedToLoadPasswordCredentialMsg:  "載入密碼憑證失敗",
		failedToRefreshSessionUserStateMsg: "重新整理工作階段的使用者狀態失敗",
	})
}
