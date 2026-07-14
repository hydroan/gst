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

	model.Base
}

// AdminUserGetRsp returns a tenant-visible user for privileged administrators.
type AdminUserGetRsp struct {
	User AdminUserView `json:"user"`
}

// AdminUserListRsp returns tenant-visible users for privileged administrators.
type AdminUserListRsp struct {
	Items []AdminUserView `json:"items"`
	Total int             `json:"total"`
}

// AdminUserView describes an IAM user for privileged administrator APIs.
type AdminUserView struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Email              string     `json:"email,omitempty"`
	Status             UserStatus `json:"status"`
	MustChangePassword bool       `json:"must_change_password"`
	CreatedAt          time.Time  `json:"created_at,omitzero"`
	UpdatedAt          time.Time  `json:"updated_at,omitzero"`
}

// UserStatusPatchReq is the request payload for changing a user's lifecycle status.
type UserStatusPatchReq struct {
	Status UserStatus `json:"status" validate:"required"`
}

// UserStatusPatchRsp returns the user status update result.
type UserStatusPatchRsp struct {
	Msg string `json:"msg,omitempty"`
}

func (User) Design() {
	Migrate()

	Route("/iam/admin/users", func() {
		List(func() {
			Service()
			Flatten()
			Filename("list.go")
			Result[*AdminUserListRsp]()
		})
		Get(func() {
			Service()
			Flatten()
			Filename("get.go")
			Result[*AdminUserGetRsp]()
		})
	})

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
