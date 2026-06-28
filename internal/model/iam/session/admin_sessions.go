package modeliamsession

// AdminSessionsListReq is the request payload for listing all sessions grouped by user.
type AdminSessionsListReq struct{}

// AdminSessionsListRsp returns all active sessions grouped by user for privileged administrators.
type AdminSessionsListRsp struct {
	Items        []AdminSessionUserView `json:"items"`
	Total        int64                  `json:"total"`
	SessionTotal int64                  `json:"session_total"`
}

// AdminSessionsGetReq is the request payload for loading a specified session as a privileged administrator.
type AdminSessionsGetReq struct{}

// AdminSessionsGetRsp returns the detail of a specified session for a privileged administrator.
type AdminSessionsGetRsp struct {
	Session SessionView `json:"session"`
}

// AdminSessionsDeleteReq is the request payload for deleting a specified session as a privileged administrator.
type AdminSessionsDeleteReq struct{}

// AdminSessionsDeleteRsp returns the result of deleting a specified session for a privileged administrator.
type AdminSessionsDeleteRsp struct{}
