package modeliamaccount

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// PasswordCredential stores password authentication state for an IAM user.
type PasswordCredential struct {
	UserID             string     `json:"user_id" query:"user_id" gorm:"type:char(36);uniqueIndex;not null"`
	PasswordHash       string     `json:"-" binding:"required"`
	MustChangePassword bool       `json:"must_change_password"`
	FailedLoginCount   int        `json:"failed_login_count"`
	LockedUntil        *time.Time `json:"locked_until,omitempty"`
	PasswordChangedAt  *time.Time `json:"password_changed_at,omitempty"`

	model.Base
}

func (PasswordCredential) Design() {
	Migrate()
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
