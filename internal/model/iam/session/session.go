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

// SessionListRsp returns all active sessions of the current authenticated user.
type SessionListRsp struct {
	Items []SessionView `json:"items"`
	Total int           `json:"total"`
}

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
			Flatten()
			Service("session_list")
			Result[*SessionListRsp]()
		})

		Get(func() {
			Flatten()
			Service("session_get")
			Result[*SessionGetRsp]()
		})

		Delete(func() {
			Flatten()
			Service("session_delete")
			Payload[*SessionDeleteReq]()
			Result[*SessionDeleteRsp]()
		})

		Delete(func() {
			Flatten()
			Exact()
			Service("session_delete_all")
			Payload[*SessionDeleteAllReq]()
			Result[*SessionDeleteAllRsp]()
		})
	})
}
