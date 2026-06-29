package modeliamsession

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sessions struct {
	model.Empty
}

// SessionsListReq is the request payload for listing active sessions of the current user.
type SessionsListReq struct{}

// SessionsListRsp returns all active sessions of the current authenticated user.
type SessionsListRsp struct {
	Items []SessionView `json:"items"`
	Total int64         `json:"total"`
}

// SessionsGetReq is the request payload for loading a specified session of the current user.
type SessionsGetReq struct{}

// SessionsGetRsp returns the detail of a specified session of the current authenticated user.
type SessionsGetRsp struct {
	Session SessionView `json:"session"`
}

// SessionsDeleteReq is the request payload for deleting a specified session of the current user.
type SessionsDeleteReq struct{}

// SessionsDeleteRsp returns the delete result for a specified session of the current user.
type SessionsDeleteRsp struct{}

// SessionsDeleteAllReq is the request payload for deleting all sessions of the current user.
type SessionsDeleteAllReq struct{}

// SessionsDeleteAllRsp returns the delete result for all sessions of the current user.
type SessionsDeleteAllRsp struct{}

func (Sessions) Design() {
	Route("/iam/sessions", func() {
		List(func() {
			Service()
			Flatten()
			Filename("sessions.go")
			Payload[*SessionsListReq]()
			Result[*SessionsListRsp]()
		})

		Get(func() {
			Service()
			Flatten()
			Filename("sessions.go")
			Payload[*SessionsGetReq]()
			Result[*SessionsGetRsp]()
		})

		Delete(func() {
			Service()
			Flatten()
			Filename("sessions.go")
			Payload[*SessionsDeleteReq]()
			Result[*SessionsDeleteRsp]()
		})

		Delete(func() {
			Service()
			Flatten()
			Exact()
			Filename("sessions.go")
			Payload[*SessionsDeleteAllReq]()
			Result[*SessionsDeleteAllRsp]()
		})
	})
}
