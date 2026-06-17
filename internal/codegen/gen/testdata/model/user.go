package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type User struct {
	Name string
	Age  int

	model.Base
}

type UserCreateRequest struct {
	CustomField1 string
	CustomField2 *string
}
type UserUpdateResponse struct {
	CustomField3 int
	CustomField4 any
}

func (*User) Design() {
	Create(func() {
		Payload[UserCreateRequest]()
		Result[*User]()
	})

	Update(func() {
		Payload[User]()
		Result[*UserUpdateResponse]()
	})

	Patch(func() {
		Payload[User]()
		Result[*User]()
	})

	UpdateMany(func() {
		Payload[User]()
		Result[User]()
	})

	DeleteMany(func() {
		Payload[*User]()
		Result[*User]()
	})
}
