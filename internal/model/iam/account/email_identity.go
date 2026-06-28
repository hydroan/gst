package modeliamaccount

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// EmailIdentity stores the primary email identity for an IAM user.
type EmailIdentity struct {
	UserID          string     `json:"user_id" schema:"user_id" gorm:"size:191;uniqueIndex;not null"`
	Email           string     `json:"email" schema:"email" gorm:"size:191;not null"`
	NormalizedEmail string     `json:"normalized_email" schema:"normalized_email" gorm:"size:191;uniqueIndex;not null"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	LastChangedAt   *time.Time `json:"last_changed_at,omitempty"`

	model.Base
}

func (EmailIdentity) Design() {
	Migrate(true)
}

func (EmailIdentity) Purge() bool { return true }
