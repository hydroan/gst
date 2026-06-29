package modeliamsession

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type AdminUserSessions struct {
	model.Empty
}

// AdminUserSessionsListReq is the request payload for loading all sessions of a specified user as a privileged administrator.
type AdminUserSessionsListReq struct{}

// AdminUserSessionsListRsp returns all sessions of a specified user for a privileged administrator.
type AdminUserSessionsListRsp struct {
	User AdminSessionOwnerView `json:"user"`
}

// AdminUserSessionsDeleteReq is the request payload for invalidating all sessions of a specified user as a privileged administrator.
type AdminUserSessionsDeleteReq struct{}

// AdminUserSessionsDeleteRsp returns the result of invalidating all sessions of a specified user for a privileged administrator.
type AdminUserSessionsDeleteRsp struct{}

func (AdminUserSessions) Design() {
	Route("/iam/admin/users/:id/session", func() {
		List(func() {
			Service()
			Flatten()
			Filename("admin_user_session.go")
			Payload[*AdminUserSessionsListReq]()
			Result[*AdminUserSessionsListRsp]()
		})

		Delete(func() {
			Service()
			Flatten()
			Exact()
			Filename("admin_user_session.go")
			Payload[*AdminUserSessionsDeleteReq]()
			Result[*AdminUserSessionsDeleteRsp]()
		})
	})
}
