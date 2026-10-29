package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Preference is one setting of the caller's, named by a key the caller
// holds once: the unique index on the owner and the key declares it, and the
// set route derives the row's id from the two. Setting a key again replaces
// its value in place, an upsert in service/preference, so the client never
// asks whether the key exists; List and Delete are the framework's own with
// a hook each, keeping the list to the caller's and refusing to delete
// someone else's. GET /api/preferences, POST /api/preferences/set, DELETE
// /api/preferences/:id.
type Preference struct {
	// UserID is the owner, taken from the session when the preference is set.
	UserID string `json:"user_id" query:"user_id" gorm:"size:36;not null"`
	Key    string `json:"key" query:"key" gorm:"size:64;not null"`
	Value  string `json:"value" gorm:"size:191"`

	model.Base
}

func (Preference) TableName() string { return "preferences" }

// Indexes declares that a key is held once per owner.
func (Preference) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"UserID", "Key"}, Unique: true}}
}

func (Preference) Design() {
	Migrate()
	Endpoint("preferences")

	Delete(func() {
		Service()
	})
	List(func() {
		Service()
	})

	Route("preferences/set", func() {
		Create(func() {
			Service("set")
			Payload[*PreferenceSetReq]()
			Result[*PreferenceSetRsp]()
		})
	})
}

type (
	// PreferenceSetReq is the key to set and its value.
	PreferenceSetReq struct {
		Key   string `json:"key" binding:"required,max=64"`
		Value string `json:"value" binding:"max=191"`
	}

	// PreferenceSetRsp is the preference as stored, the row the key already
	// had or the one written for it.
	PreferenceSetRsp = Preference
)
