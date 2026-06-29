package modeliamsession

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// AdminSession declares administrator APIs for sessions across users.
type AdminSession struct {
	model.Empty
}

// AdminSessionListReq is the request payload for listing all sessions grouped by user.
type AdminSessionListReq struct{}

// AdminSessionListRsp returns all active sessions grouped by user for privileged administrators.
type AdminSessionListRsp struct {
	Items        []AdminSessionOwnerView `json:"items"`
	Total        int64                   `json:"total"`
	SessionTotal int64                   `json:"session_total"`
}

// AdminSessionGetReq is the request payload for loading a specified session as a privileged administrator.
type AdminSessionGetReq struct{}

// AdminSessionGetRsp returns the detail of a specified session for a privileged administrator.
type AdminSessionGetRsp struct {
	Session SessionView `json:"session"`
}

// AdminSessionDeleteReq is the request payload for deleting a specified session as a privileged administrator.
type AdminSessionDeleteReq struct{}

// AdminSessionDeleteRsp returns the result of deleting a specified session for a privileged administrator.
type AdminSessionDeleteRsp struct{}

func (AdminSession) Design() {
	Route("/iam/admin/sessions", func() {
		List(func() {
			Service()
			Flatten()
			Filename("admin_session.go")
			Payload[*AdminSessionListReq]()
			Result[*AdminSessionListRsp]()
		})
		Get(func() {
			Service()
			Flatten()
			Filename("admin_session.go")
			Payload[*AdminSessionGetReq]()
			Result[*AdminSessionGetRsp]()
		})
		Delete(func() {
			Service()
			Flatten()
			Filename("admin_session.go")
			Payload[*AdminSessionDeleteReq]()
			Result[*AdminSessionDeleteRsp]()
		})
	})
}
