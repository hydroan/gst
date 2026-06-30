package modeliamprofile

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/datatypes"
)

// Profile stores the generic self-service profile data for an IAM user.
type Profile struct {
	UserID      string            `json:"user_id" schema:"user_id" gorm:"type:char(36);uniqueIndex;not null"`
	DisplayName string            `json:"display_name,omitempty" schema:"display_name" gorm:"size:191"`
	FirstName   string            `json:"first_name,omitempty" schema:"first_name" gorm:"size:191"`
	LastName    string            `json:"last_name,omitempty" schema:"last_name" gorm:"size:191"`
	Avatar      string            `json:"avatar,omitempty" schema:"avatar" gorm:"size:512"`
	Metadata    datatypes.JSONMap `json:"metadata,omitempty"`

	model.Base
}

// ProfileGetReq is the request payload for getting the current user's profile.
type ProfileGetReq struct{}

// ProfileGetRsp returns the current user's profile.
type ProfileGetRsp = Profile

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

func (Profile) Design() {
	Migrate(true)
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

func (Profile) Purge() bool { return true }
