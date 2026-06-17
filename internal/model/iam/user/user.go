package modeliamuser

import (
	"encoding/json"
	"time"

	. "github.com/hydroan/gst/dsl"
	modeliamgroup "github.com/hydroan/gst/internal/model/iam/group"
	modeliamtenant "github.com/hydroan/gst/internal/model/iam/tenant"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"golang.org/x/crypto/bcrypt"
)

// UserStatus is the account lifecycle state for IAM users.
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusInactive UserStatus = "inactive"
	UserStatusLocked   UserStatus = "locked"
)

// UserType defines IAM user categories.
type UserType string

const (
	UserTypeRegular  UserType = "regular"
	UserTypeAdmin    UserType = "admin"
	UserTypeSystem   UserType = "system"
	UserTypeMerchant UserType = "merchant"
	UserTypeGuest    UserType = "guest"
)

type UserReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`

	Status           UserStatus `json:"status"`
	Type             UserType   `json:"type"`
	GroupID          string     `json:"group_id"`
	Avatar           string     `json:"avatar"`
	TwoFactorEnabled bool       `json:"two_factor_enabled"`
	IsSuperuser      bool       `json:"is_superuser"`
}

// UserPatchReq is the allow-listed request payload for patching IAM user profile fields.
// It intentionally excludes security-sensitive fields such as username, status, password,
// superuser flags, verification state, and login counters.
type UserPatchReq struct {
	GroupID     *string    `json:"group_id,omitempty"`
	Email       *string    `json:"email,omitempty"`
	Phone       *string    `json:"phone,omitempty"`
	FirstName   *string    `json:"first_name,omitempty"`
	LastName    *string    `json:"last_name,omitempty"`
	DisplayName *string    `json:"display_name,omitempty"`
	Avatar      *string    `json:"avatar,omitempty"`
	Bio         *string    `json:"bio,omitempty"`
	Birthday    *time.Time `json:"birthday,omitempty"`
	Gender      *string    `json:"gender,omitempty"`
	TenantID    *string    `json:"tenant_id,omitempty"`
}

type User struct {
	Username string               `json:"username" gorm:"type:varchar(50);uniqueIndex;not null"`
	Status   UserStatus           `json:"status" gorm:"type:varchar(20);default:'active';index"`
	Type     UserType             `json:"type" gorm:"type:varchar(20);default:'regular';index"`
	GroupID  string               `json:"group_id" gorm:"type:varchar(100);index"`
	Group    *modeliamgroup.Group `json:"group,omitempty" gorm:"-"`

	Email       *string    `json:"email" gorm:"type:varchar(100);uniqueIndex"`
	Phone       *string    `json:"phone" gorm:"type:varchar(20);index"`
	FirstName   *string    `json:"first_name" gorm:"type:varchar(50)"`
	LastName    *string    `json:"last_name" gorm:"type:varchar(50)"`
	DisplayName *string    `json:"display_name" gorm:"type:varchar(100)"`
	Avatar      *string    `json:"avatar" gorm:"type:varchar(500)"`
	Bio         *string    `json:"bio" gorm:"type:varchar(500)"`
	Birthday    *time.Time `json:"birthday"`
	Gender      *string    `json:"gender" gorm:"type:varchar(10)"`

	Password           string `json:"password,omitempty" gorm:"-"`
	PasswordHash       string `json:"-" gorm:"type:varchar(255)"`
	Salt               string `json:"-" gorm:"type:varchar(50)"`
	TwoFactorEnabled   *bool  `json:"two_factor_enabled" gorm:"default:false"`
	MustChangePassword bool   `json:"must_change_password" gorm:"default:false;not null"`

	EmailVerified      *bool      `json:"email_verified" gorm:"default:false"`
	EmailVerifiedAt    *time.Time `json:"email_verified_at"`
	PhoneVerified      *bool      `json:"phone_verified" gorm:"default:false"`
	LastEmailChangedAt *time.Time `json:"last_email_changed_at"`

	IsStaff     *bool `json:"is_staff" gorm:"default:false"`
	IsSuperuser *bool `json:"is_superuser" gorm:"default:false"`

	TenantID *string                `json:"tenant_id" gorm:"index"`
	Tenant   *modeliamtenant.Tenant `json:"tenant,omitempty" gorm:"-"`

	LastLoginAt      *time.Time `json:"last_login_at"`
	LastLoginIP      *string    `json:"last_login_ip" gorm:"type:varchar(45)"`
	LoginCount       *int       `json:"login_count" gorm:"default:0"`
	FailedLoginCount int        `json:"failed_login_count" gorm:"default:0"`
	LockedUntil      *time.Time `json:"locked_until"`

	model.Base
}

// MarshalJSON keeps write-only credential fields out of API responses.
func (u User) MarshalJSON() ([]byte, error) {
	type userResponse struct {
		Username string               `json:"username"`
		Status   UserStatus           `json:"status"`
		Type     UserType             `json:"type"`
		GroupID  string               `json:"group_id"`
		Group    *modeliamgroup.Group `json:"group,omitempty"`

		Email       *string    `json:"email"`
		Phone       *string    `json:"phone"`
		FirstName   *string    `json:"first_name"`
		LastName    *string    `json:"last_name"`
		DisplayName *string    `json:"display_name"`
		Avatar      *string    `json:"avatar"`
		Bio         *string    `json:"bio"`
		Birthday    *time.Time `json:"birthday"`
		Gender      *string    `json:"gender"`

		TwoFactorEnabled   *bool `json:"two_factor_enabled"`
		MustChangePassword bool  `json:"must_change_password"`

		EmailVerified      *bool      `json:"email_verified"`
		EmailVerifiedAt    *time.Time `json:"email_verified_at"`
		PhoneVerified      *bool      `json:"phone_verified"`
		LastEmailChangedAt *time.Time `json:"last_email_changed_at"`

		IsStaff     *bool `json:"is_staff"`
		IsSuperuser *bool `json:"is_superuser"`

		TenantID *string                `json:"tenant_id"`
		Tenant   *modeliamtenant.Tenant `json:"tenant,omitempty"`

		LastLoginAt      *time.Time `json:"last_login_at"`
		LastLoginIP      *string    `json:"last_login_ip"`
		LoginCount       *int       `json:"login_count"`
		FailedLoginCount int        `json:"failed_login_count"`
		LockedUntil      *time.Time `json:"locked_until"`

		model.Base
	}

	return json.Marshal(userResponse{
		Username:           u.Username,
		Status:             u.Status,
		Type:               u.Type,
		GroupID:            u.GroupID,
		Group:              u.Group,
		Email:              u.Email,
		Phone:              u.Phone,
		FirstName:          u.FirstName,
		LastName:           u.LastName,
		DisplayName:        u.DisplayName,
		Avatar:             u.Avatar,
		Bio:                u.Bio,
		Birthday:           u.Birthday,
		Gender:             u.Gender,
		TwoFactorEnabled:   u.TwoFactorEnabled,
		MustChangePassword: u.MustChangePassword,
		EmailVerified:      u.EmailVerified,
		EmailVerifiedAt:    u.EmailVerifiedAt,
		PhoneVerified:      u.PhoneVerified,
		LastEmailChangedAt: u.LastEmailChangedAt,
		IsStaff:            u.IsStaff,
		IsSuperuser:        u.IsSuperuser,
		TenantID:           u.TenantID,
		Tenant:             u.Tenant,
		LastLoginAt:        u.LastLoginAt,
		LastLoginIP:        u.LastLoginIP,
		LoginCount:         u.LoginCount,
		FailedLoginCount:   u.FailedLoginCount,
		LockedUntil:        u.LockedUntil,
		Base:               u.Base,
	})
}

func (User) Design() {
	Migrate(true)
	Endpoint("users")

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
		Service(true)
	})
	Get(func() {
		Enabled(true)
		Service(true)
	})
}

func (User) Purge() bool { return true }

func (u *User) CreateBefore(ctx *types.ModelContext) error { return GenerateHashedPassword(u) }
func (u *User) UpdateBefore(ctx *types.ModelContext) error { return GenerateHashedPassword(u) }

func GenerateHashedPassword(u *User) error {
	if len(u.Password) > 0 && len(u.PasswordHash) == 0 {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.PasswordHash = string(hashedPassword)
		return nil
	}
	return nil
}
