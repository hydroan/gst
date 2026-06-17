package modeliamsession

// AdminUserSessionsListReq is the request payload for loading all sessions of a specified user as a privileged administrator.
type AdminUserSessionsListReq struct{}

// AdminUserSessionsListRsp returns all sessions of a specified user for a privileged administrator.
type AdminUserSessionsListRsp struct {
	User AdminSessionUserView `json:"user"`
}

// AdminUserSessionsDeleteReq is the request payload for invalidating all sessions of a specified user as a privileged administrator.
type AdminUserSessionsDeleteReq struct{}

// AdminUserSessionsDeleteRsp returns the result of invalidating all sessions of a specified user for a privileged administrator.
type AdminUserSessionsDeleteRsp struct{}
