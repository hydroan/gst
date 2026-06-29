package modeliamsession

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// AdminUserSessionDelete declares administrator APIs for sessions owned by a specified user.
type AdminUserSessionDelete struct {
	model.Empty
}

// AdminUserSessionDeleteReq is the request payload for invalidating all sessions of a specified user as a privileged administrator.
type AdminUserSessionDeleteReq struct{}

// AdminUserSessionDeleteRsp returns the result of invalidating all sessions of a specified user for a privileged administrator.
type AdminUserSessionDeleteRsp struct{}

func (AdminUserSessionDelete) Design() {
	Route("/iam/admin/users/:id/sessions", func() {
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
