package modelmfa

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/datatypes"
)

// TOTPDevice stores a registered TOTP authenticator for an IAM user.
//
// The model is registered for storage only. Device management is exposed through
// dedicated MFA actions instead of default CRUD routes so sensitive fields stay
// behind service-level checks.
//
// IsActive carries no column default on purpose: GORM substitutes a declared
// default for every zero value at insert time, which would turn a device staged
// before code verification into an already verified one. Every caller states the
// value it wants.
type TOTPDevice struct {
	UserID           string                      `json:"user_id" gorm:"type:varchar(191);not null;index" query:"user_id"`
	DeviceName       string                      `json:"device_name" gorm:"type:varchar(100);not null" query:"device_name"`
	Secret           string                      `json:"-" query:"secret"` // Base32 encoded secret, not exposed in JSON
	BackupCodeHashes datatypes.JSONSlice[string] `json:"-" query:"backup_code_hashes"`
	IsActive         bool                        `json:"is_active" query:"is_active"`
	LastUsedAt       *time.Time                  `json:"last_used_at" query:"last_used_at"`

	model.Base
}

func (TOTPDevice) Design() {
	Migrate()
}
