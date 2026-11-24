package modeliamsession

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Current struct {
	model.Empty
}

// CurrentGetRsp returns the current session together with the latest principal snapshot.
type CurrentGetRsp = AuthenticatedSessionRsp

// CurrentDeleteReq is the request payload for deleting the current session.
type CurrentDeleteReq struct{}

// CurrentDeleteRsp is the response payload for deleting the current session.
type CurrentDeleteRsp struct{}

func (Current) Design() {
	Route("/iam/session/current", func() {
		Get(func() {
			Flatten()
			Exact()
			Service("current_get")
			Result[*CurrentGetRsp]()
		})
		Delete(func() {
			Flatten()
			Exact()
			Service("current_delete")
			Payload[*CurrentDeleteReq]()
			Result[*CurrentDeleteRsp]()
		})
	})
}
