package modeltwofa

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"gorm.io/datatypes"
)

// TOTPDevice represents a TOTP device for 2FA
type TOTPDevice struct {
	UserID      string                      `json:"user_id" gorm:"type:varchar(191);not null;index" schema:"user_id"`
	DeviceName  string                      `json:"device_name" gorm:"type:varchar(100);not null" schema:"device_name"`
	Secret      string                      `json:"-" schema:"secret"`       // Base32 encoded secret, not exposed in JSON
	BackupCodes datatypes.JSONSlice[string] `json:"-" schema:"backup_codes"` // JSON array of backup codes
	IsActive    bool                        `json:"is_active" gorm:"default:true" schema:"is_active"`
	LastUsedAt  *time.Time                  `json:"last_used_at" schema:"last_used_at"`

	model.Base
}

func (TOTPDevice) Design() {
	Migrate(true)

	Route("2fa/totp/devices", func() {
		Create(func() {
			Enabled(true)
		})
		Delete(func() {
			Enabled(true)
		})
		Update(func() {
			Enabled(true)
		})
		Patch(func() {
			Enabled(true)
		})
		List(func() {
			Enabled(true)
		})
		Get(func() {
			Enabled(true)
		})
	})
}
