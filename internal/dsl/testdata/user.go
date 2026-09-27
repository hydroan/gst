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
	// Migration is disabled by default; declaring Migrate() enables it.
	Migrate()
	// gRPC is off by default; declaring GRPC() serves the model over it too.
	GRPC()
	// Default Endpoint is the pluralized snake_case form of the model name.
	Endpoint("//iam/user2")
	Param("user")

	Route("/iam/users", func() {
		List(func() {
			Service()
			Payload[*UserReq]()
			Result[*UserRsp]()
		})
		Get(func() {
			Service()
		})
	})
	Route("///tenant/users", func() {
		Create(func() {
			Payload[*UserReq]()
			Result[*User]()
		})
		Update(func() {
		})
		Patch(func() {
		})
		CreateMany(func() {
		})
	})

	// Custom create action request "Payload" and response "Result".
	// Default payload and result is the model name.
	Create(func() {
		Public()
		Service()
		Payload[User]()
		Result[*User]()
	})

	// Custom update action request "Payload" and response "Result".
	Update(func() {
		Payload[*User]()
		Result[User]()
	})

	Delete(func() {
	})

	List(func() {
	})
}
