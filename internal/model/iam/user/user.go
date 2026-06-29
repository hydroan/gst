package modeliamuser

import (
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

	model.Base
}
type UserStatusPatchReq struct {
	Status UserStatus `json:"status" validate:"required"`
}

type UserStatusPatchRsp struct {
	Msg string `json:"msg,omitempty"`
}

func (User) Design() {
	Migrate(true)

	Route("/iam/admin/users/:id/status", func() {
		Patch(func() {
			Service()
			Flatten()
			Exact()
			Filename("status.go")
			Payload[*UserStatusPatchReq]()
			Result[*UserStatusPatchRsp]()
		})
	})
}

func (User) Purge() bool { return true }
