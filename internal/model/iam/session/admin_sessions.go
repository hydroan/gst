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

// AdminSessionUserView describes a user together with all indexed sessions owned by the user.
type AdminSessionUserView struct {
	UserID             string        `json:"user_id"`
	Username           string        `json:"username"`
	Email              string        `json:"email"`
	FirstName          *string       `json:"first_name,omitempty"`
	LastName           *string       `json:"last_name,omitempty"`
	GroupID            string        `json:"group_id,omitempty"`
	GroupName          string        `json:"group_name,omitempty"`
	Status             string        `json:"status"`
	MustChangePassword bool          `json:"must_change_password"`
	SessionTotal       int64         `json:"session_total"`
	Sessions           []SessionView `json:"sessions"`
}
