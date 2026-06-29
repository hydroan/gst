package modeliamprofile

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/datatypes"
)

type ProfilePatch struct {
	model.Empty
}

// ProfilePatchReq is the request payload for patching the current user's profile.
type ProfilePatchReq struct {
	DisplayName *string           `json:"display_name,omitempty"`
	FirstName   *string           `json:"first_name,omitempty"`
	LastName    *string           `json:"last_name,omitempty"`
	Avatar      *string           `json:"avatar,omitempty"`
	Metadata    datatypes.JSONMap `json:"metadata,omitempty"`
}

// ProfilePatchRsp returns the patched current user's profile.
type ProfilePatchRsp = Profile

func (ProfilePatch) Design() {
	Route("/iam/profile", func() {
		Patch(func() {
			Service()
			Flatten()
			Exact()
			Filename("patch.go")
			Payload[*ProfilePatchReq]()
			Result[*ProfilePatchRsp]()
		})
	})
}
