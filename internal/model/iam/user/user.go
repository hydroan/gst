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

// AdminUserCreateReq is the request payload for creating a user.
//
// The account is created with an identity and a way to sign in, and with
// nothing else: which tenants it belongs to and what it may do there are role
// bindings, which authz owns and this module deliberately does not write.
type AdminUserCreateReq struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
	Email    string `json:"email,omitempty"`

	// MustChangePassword defaults to true when omitted, because a password
	// someone else chose is a password two people know. Sending false is how an
	// automated caller provisioning a service account opts out.
	MustChangePassword *bool `json:"must_change_password,omitempty"`
}

// AdminUserCreateRsp returns the created user.
type AdminUserCreateRsp struct {
	User AdminUserView `json:"user"`
}

// AdminUserPatchReq is the request payload for updating a user.
//
// Every field is optional and only the ones present are written, which is what
// PATCH means; a request naming none of them is refused rather than treated as
// a no-op, because it cannot be told apart from one whose field names are
// misspelled.
type AdminUserPatchReq struct {
	Username *string     `json:"username,omitempty"`
	Status   *UserStatus `json:"status,omitempty"`
}

// AdminUserPatchRsp returns the updated user.
type AdminUserPatchRsp struct {
	User AdminUserView `json:"user"`
}

func (User) Design() {
	Migrate()

	Route("/iam/admin/users", func() {
		Create(func() {
			Service()
			Flatten()
			Payload[*AdminUserCreateReq]()
			Result[*AdminUserCreateRsp]()
		})
		List(func() {
			Service()
			Flatten()
			Result[*AdminUserListRsp]()
		})
		Get(func() {
			Service()
			Flatten()
			Result[*AdminUserGetRsp]()
		})
		Patch(func() {
			Service()
			Flatten()
			Payload[*AdminUserPatchReq]()
			Result[*AdminUserPatchRsp]()
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
