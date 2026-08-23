package modeliamaccount

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// PasswordCredential stores password authentication state for an IAM user.
//
// Failed attempts are not counted here. A lockout has to be atomic under
// concurrent attempts and has to expire on its own, and a row read back,
// incremented, and written would deliver neither; the counter lives in Redis,
// where an increment is one operation and a window is a ttl.
type PasswordCredential struct {
	UserID             string     `json:"user_id" query:"user_id" gorm:"size:36;not null"`
	PasswordHash       string     `json:"-" binding:"required"`
	MustChangePassword bool       `json:"must_change_password"`
	PasswordChangedAt  *time.Time `json:"password_changed_at,omitempty"`

	model.Base
}

func (PasswordCredential) Design() {
	Migrate()
}

// TableName pins the table name gorm would otherwise derive.
func (PasswordCredential) TableName() string { return "password_credentials" }

// Indexes declares one credential row per user; Purge explains why that
// uniqueness requires hard deletion.
func (PasswordCredential) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"UserID"}, Unique: true},
	}
}

// Purge hard-deletes, which is what the row's own uniqueness requires.
//
// One user holds one credential, and the unique index on user_id says so. A
// soft-deleted row keeps that index slot, so the deletion frees nothing: a
// later credential for the same user collides with a row the framework already
// considers gone, and reports a duplicate key for a conflict nothing can see.
//
// Nothing reads a deleted credential either. It carries no history worth
// keeping — a password hash, a failure count, a lock expiry — all of which
// describe a login that can no longer happen, while the hash is the one field
// least worth leaving behind.
func (PasswordCredential) Purge() bool { return true }
