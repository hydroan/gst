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
//
// The (user_id, secret) unique index is the authoritative guard against binding
// the same secret twice for one user; the service-level duplicate check is only
// a friendlier fast path. The index assumes hard deletion (see Purge): a
// soft-deleted row would keep occupying the unique key.
type TOTPDevice struct {
	UserID           string                      `json:"user_id" gorm:"type:varchar(191);not null;index;uniqueIndex:idx_totp_devices_user_id_secret,priority:1"`
	DeviceName       string                      `json:"device_name" gorm:"type:varchar(100);not null"`
	Secret           string                      `json:"-" gorm:"type:varchar(64);not null;uniqueIndex:idx_totp_devices_user_id_secret,priority:2"` // Base32 encoded secret (52 chars at SecretSize 32), not exposed in JSON
	BackupCodeHashes datatypes.JSONSlice[string] `json:"-"`
	IsActive         bool                        `json:"is_active"`
	LastUsedAt       *time.Time                  `json:"last_used_at"`

	model.Base
}

// Purge makes every TOTPDevice deletion a hard delete. An unbound device must
// not leave its shared secret and recovery-code hashes behind as a soft-deleted
// row, and the (user_id, secret) unique index relies on removed rows actually
// releasing their key.
func (TOTPDevice) Purge() bool { return true }

func (TOTPDevice) Design() {
	Migrate()
}
