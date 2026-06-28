package modeliamuser

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// UserStatus is the account lifecycle state for IAM users.
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusInactive UserStatus = "inactive"
	UserStatusLocked   UserStatus = "locked"
)

type User struct {
	Username string     `json:"username" gorm:"type:varchar(50);uniqueIndex;not null"`
	Status   UserStatus `json:"status" gorm:"type:varchar(20);default:'active';index"`

	IsSuperuser *bool `json:"is_superuser" gorm:"default:false"`

	LastLoginAt *time.Time `json:"last_login_at"`
	LastLoginIP *string    `json:"last_login_ip" gorm:"type:varchar(45)"`
	LoginCount  *int       `json:"login_count" gorm:"default:0"`

	model.Base
}

func (User) Design() {
	Migrate(true)
	Endpoint("users")
}

func (User) Purge() bool { return true }
