package modeliamprofile

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type ProfileGet struct {
	model.Empty
}

// ProfileGetReq is the request payload for getting the current user's profile.
type ProfileGetReq struct{}

// ProfileGetRsp returns the current user's profile.
type ProfileGetRsp = Profile

func (ProfileGet) Design() {
	Route("/iam/profile", func() {
		Get(func() {
			Service()
			Flatten()
			Exact()
			Filename("get.go")
			Payload[*ProfileGetReq]()
			Result[*ProfileGetRsp]()
		})
	})
}
