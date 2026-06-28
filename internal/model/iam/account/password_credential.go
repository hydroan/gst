package modeliamaccount

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// PasswordCredential stores password authentication state for an IAM user.
type PasswordCredential struct {
	model.Base

	UserID             string     `json:"user_id" gorm:"uniqueIndex;not null" binding:"required"`
	PasswordHash       string     `json:"-" binding:"required"`
	MustChangePassword bool       `json:"must_change_password"`
	FailedLoginCount   int        `json:"failed_login_count"`
	LockedUntil        *time.Time `json:"locked_until,omitempty"`
	PasswordChangedAt  *time.Time `json:"password_changed_at,omitempty"`
}

func (PasswordCredential) Design() {
	Migrate(true)
}
