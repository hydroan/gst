package modeliamprofile

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/datatypes"
)

// Profile stores the generic self-service profile data for an IAM user.
type Profile struct {
	UserID      string            `json:"user_id" query:"user_id" gorm:"type:char(36);uniqueIndex;not null"`
	DisplayName string            `json:"display_name,omitempty" query:"display_name" gorm:"size:191"`
	FirstName   string            `json:"first_name,omitempty" query:"first_name" gorm:"size:191"`
	LastName    string            `json:"last_name,omitempty" query:"last_name" gorm:"size:191"`
	Avatar      string            `json:"avatar,omitempty" query:"avatar" gorm:"size:512"`
	Metadata    datatypes.JSONMap `json:"metadata,omitempty"`

	model.Base
}

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
	Migrate()
	Route("/iam/profile", func() {
		Get(func() {
			Service()
			Flatten()
			Exact()
			Filename("get.go")
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
