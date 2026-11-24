package modeliamaccount

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// EmailIdentity stores the primary email identity for an IAM user.
type EmailIdentity struct {
	UserID          string     `json:"user_id" query:"user_id" gorm:"size:36;not null"`
	Email           string     `json:"email" query:"email" gorm:"size:191;not null"`
	NormalizedEmail string     `json:"normalized_email" query:"normalized_email" gorm:"size:191;not null"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	LastChangedAt   *time.Time `json:"last_changed_at,omitempty"`

	model.Base
}

func (EmailIdentity) Design() {
	Migrate()
}

// TableName pins the table name gorm would otherwise derive.
func (EmailIdentity) TableName() string { return "email_identities" }

// Indexes declares one identity row per user and the global uniqueness of
// normalized email addresses used for lookup and duplicate detection.
func (EmailIdentity) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"UserID"}, Unique: true},
		{Fields: []string{"NormalizedEmail"}, Unique: true},
	}
}

func (EmailIdentity) Purge() bool { return true }
