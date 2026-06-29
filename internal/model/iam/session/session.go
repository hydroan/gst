package modeliamsession

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Session2 declares the self-service session API routes. The suffix avoids
// colliding with Session, the stored session snapshot.
type Session2 struct {
	model.Empty
}

// SessionListReq is the request payload for listing active sessions of the current user.
type SessionListReq struct{}

// SessionListRsp returns all active sessions of the current authenticated user.
type SessionListRsp struct {
	Items []SessionView `json:"items"`
	Total int64         `json:"total"`
}

// SessionGetReq is the request payload for loading a specified session of the current user.
type SessionGetReq struct{}

// SessionGetRsp returns the detail of a specified session of the current authenticated user.
type SessionGetRsp struct {
	Session SessionView `json:"session"`
}

// SessionDeleteReq is the request payload for deleting a specified session of the current user.
type SessionDeleteReq struct{}

// SessionDeleteRsp returns the delete result for a specified session of the current user.
type SessionDeleteRsp struct{}

// SessionDeleteAllReq is the request payload for deleting all sessions of the current user.
type SessionDeleteAllReq struct{}

// SessionDeleteAllRsp returns the delete result for all sessions of the current user.
type SessionDeleteAllRsp struct{}

func (Session2) Design() {
	Route("/iam/sessions", func() {
		List(func() {
			Service()
			Flatten()
			Filename("session_list.go")
			Payload[*SessionListReq]()
			Result[*SessionListRsp]()
		})

		Get(func() {
			Service()
			Flatten()
			Filename("session_get.go")
			Payload[*SessionGetReq]()
			Result[*SessionGetRsp]()
		})

		Delete(func() {
			Service()
			Flatten()
			Filename("session_delete.go")
			Payload[*SessionDeleteReq]()
			Result[*SessionDeleteRsp]()
		})

		Delete(func() {
			Service()
			Flatten()
			Exact()
			Filename("session_delete_all.go")
			Payload[*SessionDeleteAllReq]()
			Result[*SessionDeleteAllRsp]()
		})
	})
}
