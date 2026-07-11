package apidocmodel

// User is the user record.
type User struct {
	// Name is the user name.
	Name string `json:"name"`
	Age  int    `json:"age"` // Age is the user age.
}

// UserCreateReq is the create user request.
type UserCreateReq struct {
	// Name is the user name to create.
	Name string `json:"name"`
}

type plain struct {
	Value string
}

// UserStatus is the lifecycle status of a user.
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"   // the user can log in
	UserStatusDisabled UserStatus = "disabled" // the user is blocked
)
