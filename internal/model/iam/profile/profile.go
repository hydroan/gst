package modeliamprofile

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/datatypes"
)

// Profile stores the generic self-service profile data for an IAM user.
//
// A name is one field here, not a given one and a family one. Splitting it
// assumes an ordering and a structure that plenty of the world's names do not
// have, and nothing in the framework reads the halves separately; a project
// that needs its own decomposition can put it in Metadata, which exists for
// exactly the fields the framework cannot know about.
type Profile struct {
	UserID      string            `json:"user_id" query:"user_id" gorm:"size:36;not null"`
	DisplayName string            `json:"display_name,omitempty" query:"display_name" gorm:"size:191"`
	Avatar      string            `json:"avatar,omitempty" query:"avatar" gorm:"size:512"`
	Metadata    datatypes.JSONMap `json:"metadata,omitempty"`

	model.Base
}

// ProfileGetRsp returns the current user's profile.
type ProfileGetRsp = Profile

// ProfilePatchReq is the request payload for patching the current user's profile.
type ProfilePatchReq struct {
	DisplayName *string           `json:"display_name,omitempty"`
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
			Filename("get.go")
			Exact()
			Result[*ProfileGetRsp]()
		})
	})
	Route("/iam/profile", func() {
		Patch(func() {
			Service()
			Flatten()
			Filename("patch.go")
			Exact()
			Payload[*ProfilePatchReq]()
			Result[*ProfilePatchRsp]()
		})
	})
}

// TableName pins the table name gorm would otherwise derive.
func (Profile) TableName() string { return "profiles" }

// Indexes declares one profile row per user.
func (Profile) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"UserID"}, Unique: true},
	}
}

func (Profile) Purge() bool { return true }
