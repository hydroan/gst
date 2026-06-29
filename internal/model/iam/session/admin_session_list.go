package modeliamsession

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type AdminSessionList struct {
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

func (AdminSessionList) Design() {
	Route("/iam/admin/sessions", func() {
		List(func() {
			Service()
			Flatten()
			Filename("admin_session_list.go")
			Payload[*AdminSessionListReq]()
			Result[*AdminSessionListRsp]()
		})
	})
}
