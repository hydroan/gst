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
	// Default to true.
	Enabled(true)

	// Default Endpoint is the lower case of the model name.
	Endpoint("user")

	// Custom create action request "Payload" and response "Result".
	Create(func() {
		Enabled(false)
		Payload[User]()
		Result[*User]()
	})

	// Custom update action request "Payload" and response "Result".
	Update(func() {
		Enabled(true)
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
	// Default to true.
	dsl.Enabled(true)

	// Default Endpoint is the lower case of the model name.
	// dsl.Endpoint("user4")

	// Custom create action request "Payload" and response "Result".
	dsl.Create(func() {
		dsl.Enabled(true)
		dsl.Payload[User]()
		dsl.Result[*User]()
	})

	// Custom update action request "Payload" and response "Result".
	dsl.Update(func() {
		dsl.Enabled(false)
		dsl.Payload[*User]()
		dsl.Result[User]()
	})
}
