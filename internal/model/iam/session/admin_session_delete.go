package modeliamsession

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// AdminSessionDelete declares administrator APIs for sessions across users.
type AdminSessionDelete struct {
	model.Empty
}

// AdminSessionDeleteReq is the request payload for deleting a specified session as a privileged administrator.
type AdminSessionDeleteReq struct{}

// AdminSessionDeleteRsp returns the result of deleting a specified session for a privileged administrator.
type AdminSessionDeleteRsp struct{}

func (AdminSessionDelete) Design() {
	Route("/iam/admin/sessions", func() {
		Delete(func() {
			Service()
			Flatten()
			Filename("admin_session_delete.go")
			Payload[*AdminSessionDeleteReq]()
			Result[*AdminSessionDeleteRsp]()
		})
	})
}
