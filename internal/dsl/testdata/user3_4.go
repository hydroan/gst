package model

import (
	"github.com/hydroan/gst/dsl"
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	pkgmodel "github.com/hydroan/gst/model"
)

type User3 struct {
	Name string
	Addr string

	model.Base
}

func (User3) Design() {
	// Default Endpoint is the pluralized snake_case form of the model name.
	Endpoint("user")

	// Custom create action request "Payload" and response "Result".
	Create(func() {
		Payload[User]()
		Result[*User]()
	})

	// Custom update action request "Payload" and response "Result".
	Update(func() {
		Payload[*User]()
		Result[User]()
	})
}

type User4 struct {
	Name string
	Addr string

	pkgmodel.Base
}

func (*User4) Design() {
	// Default Endpoint is the pluralized snake_case form of the model name.
	// dsl.Endpoint("user4")

	// Custom create action request "Payload" and response "Result".
	dsl.Create(func() {
		dsl.Payload[User]()
		dsl.Result[*User]()
	})

	// Custom update action request "Payload" and response "Result".
	dsl.Update(func() {
		dsl.Payload[*User]()
		dsl.Result[User]()
	})
}
