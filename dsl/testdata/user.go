package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type User struct {
	Name string
	Addr string

	model.Base
}

func (User) Design() {
	// Default to true.
	// Enabled(true)

	// Default Endpoint is the pluralized snake_case form of the model name.
	Endpoint("//iam/user2")

	// Migration is disabled by default; declaring Migrate() enables it.
	Migrate()
	Param("user")

	Route("/iam/users", func() {
		List(func() {
			Enabled(true)
			Service()
			Payload[*UserReq]()
			Result[*UserRsp]()
		})
		Get(func() {
			Enabled(true)
			Service()
		})
	})
	Route("///tenant/users", func() {
		Create(func() {
			Enabled(true)
			Payload[*UserReq]()
			Result[*User]()
		})
		Update(func() {
			Enabled(true)
		})
		Patch(func() {
			Enabled(true)
		})
		CreateMany(func() {
			Enabled(true)
		})
	})

	// Custom create action request "Payload" and response "Result".
	// Default payload and result is the model name.
	Create(func() {
		Enabled(true)
		Service()
		Public()
		Payload[User]()
		Result[*User]()
	})

	// Custom update action request "Payload" and response "Result".
	Update(func() {
		Enabled(false)
		Payload[*User]()
		Result[User]()
	})

	Delete(func() {
		Enabled(true)
	})

	List(func() {
		Enabled(true)
	})
}
