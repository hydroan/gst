package reflectmeta_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/reflectmeta"
)

type User2 struct {
	Name         string `json:"name,omitempty"`
	EnName       string `json:"en_name,omitempty"`
	Password     string `json:"password,omitempty"`
	RePassword   string `json:"re_password,omitempty" gorm:"-"`
	NewPassword  string `json:"new_password,omitempty" gorm:"-"`
	Email        string `json:"email,omitempty"`
	Avatar       string `json:"avatar,omitempty"`
	AvatarURL    string `json:"avatar_url,omitempty"`    // 用户头像
	AvatarThumb  string `json:"avatar_thumb,omitempty"`  // 用户头像 72x72
	AvatarMiddle string `json:"avatar_middle,omitempty"` // 用户头像 240x240
	AvatarBig    string `json:"avatar_big,omitempty"`    // 用户头像 640x640
	Mobile       string `json:"mobile,omitempty"`
	Nickname     string `json:"nickname,omitempty"`
	Introduction string `json:"introduction,omitempty"`
	Status       uint   `json:"status,omitempty" gorm:"type:smallint;default:1;comment:status(0: disabled, 1: enabled)"`
	// State 员工状态
	// 1 在职
	// 2 离职
	// 3 试用期
	// 4 实习生
	RoleID       string `json:"role_id,omitempty" schema:"role_id"`
	DepartmentID string `json:"department_id,omitempty" schema:"department_id"`

	LastLoginIP string `json:"last_login_ip,omitempty"`
	LockExpire  int64  `json:"lock_expire,omitempty"`
	NumWrong    int    `json:"num_wrong,omitempty" gorm:"comment:the number of input password wrong"`

	Token        string `json:"token,omitempty" gorm:"-"`
	AccessToken  string `json:"access_token,omitempty" gorm:"-"`
	RefreshToken string `json:"refresh_token,omitempty" gorm:"-"`
	SessionID    string `json:"session_id,omitempty" gorm:"-"`

	Base
}

func BenchmarkStructFieldToMap(b *testing.B) {
	u := User2{
		Name:         "user",
		EnName:       "user",
		Password:     "mypass",
		Email:        "user@gmail.com",
		Avatar:       "avatar",
		AvatarURL:    "avatar_url",
		AvatarThumb:  "avatar_thumb",
		AvatarMiddle: "avatar_middle",
		AvatarBig:    "avatar_big",
		Mobile:       "mobile",
		Nickname:     "nickname",
		Introduction: "introduction",
		Status:       1,
		RoleID:       "role_id",
		DepartmentID: "department_id",
		LastLoginIP:  "last_login_ip",
		LockExpire:   0,
	}
	typ := reflect.TypeFor[User2]()
	val := reflect.ValueOf(u)
	q := make(map[string]string)

	b.Run("StructFieldToMap", func(b *testing.B) {
		for b.Loop() {
			reflectmeta.StructFieldToMap(typ, val, q)
		}
	})
	b.Run("StructFieldToMap2", func(b *testing.B) {
		for b.Loop() {
			reflectmeta.StructFieldToMap2(typ, val, q)
		}
	})
}
