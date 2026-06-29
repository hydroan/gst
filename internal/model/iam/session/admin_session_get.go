package modeliamsession

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// AdminSessionGet declares administrator APIs for sessions across users.
type AdminSessionGet struct {
	model.Empty
}

// AdminSessionGetReq is the request payload for loading a specified session as a privileged administrator.
type AdminSessionGetReq struct{}

// AdminSessionGetRsp returns the detail of a specified session for a privileged administrator.
type AdminSessionGetRsp struct {
	Session SessionView `json:"session"`
}

func (AdminSessionGet) Design() {
	Route("/iam/admin/sessions", func() {
		Get(func() {
			Service()
			Flatten()
			Filename("admin_session_get.go")
			Payload[*AdminSessionGetReq]()
			Result[*AdminSessionGetRsp]()
		})
	})
}
