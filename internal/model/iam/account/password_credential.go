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
	Migrate(true)
}
