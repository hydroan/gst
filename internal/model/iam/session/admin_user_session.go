package modeliamsession

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// AdminUserSession declares administrator APIs for sessions owned by a specified user.
type AdminUserSession struct {
	model.Empty
}

// AdminUserSessionListRsp returns all sessions of a specified user for a privileged administrator.
type AdminUserSessionListRsp struct {
	User AdminSessionOwnerView `json:"user"`
}

// AdminUserSessionDeleteReq is the request payload for invalidating all sessions of a specified user as a privileged administrator.
type AdminUserSessionDeleteReq struct{}

// AdminUserSessionDeleteRsp returns the result of invalidating all sessions of a specified user for a privileged administrator.
type AdminUserSessionDeleteRsp struct{}

func (AdminUserSession) Design() {
	Route("/iam/admin/users/:id/sessions", func() {
		List(func() {
			Service()
			Flatten()
			Filename("admin_user_session_list.go")
			Result[*AdminUserSessionListRsp]()
		})
		Delete(func() {
			Service()
			Flatten()
			Exact()
			Filename("admin_user_session_delete.go")
			Payload[*AdminUserSessionDeleteReq]()
			Result[*AdminUserSessionDeleteRsp]()
		})
	})
}
