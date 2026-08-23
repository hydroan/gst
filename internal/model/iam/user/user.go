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
	Username string     `json:"username" gorm:"type:varchar(50);not null"`
	Status   UserStatus `json:"status" gorm:"type:varchar(20);default:'active'"`

	// Query gives the administrator user list the framework's own paging,
	// ordering and operator filters, rather than the handful of parameters that
	// list once parsed for itself.
	//
	// Paging and ordering arrive together on purpose: a page is only meaningful
	// over an ordered set, since without an order the database may return rows
	// however it likes, and a second page drawn from a different one repeats
	// rows it already showed and skips rows it never did.
	model.Query
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

// TableName pins the table name gorm would otherwise derive.
func (User) TableName() string { return "users" }

// Indexes declares the login uniqueness of usernames and the status filter
// path used by administrator listings.
func (User) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"Username"}, Unique: true},
		{Fields: []string{"Status"}},
	}
}

func (User) Purge() bool { return true }
