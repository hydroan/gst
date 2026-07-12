package modeliamaccount

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// EmailIdentity stores the primary email identity for an IAM user.
type EmailIdentity struct {
	UserID          string     `json:"user_id" query:"user_id" gorm:"type:char(36);uniqueIndex;not null"`
	Email           string     `json:"email" query:"email" gorm:"size:191;not null"`
	NormalizedEmail string     `json:"normalized_email" query:"normalized_email" gorm:"size:191;uniqueIndex;not null"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	LastChangedAt   *time.Time `json:"last_changed_at,omitempty"`

	model.Base
}

func (EmailIdentity) Design() {
	Migrate()
}

func (EmailIdentity) Purge() bool { return true }
