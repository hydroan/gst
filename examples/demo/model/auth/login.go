package auth

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Login demonstrates a public action that is not backed by a database table.
type Login struct {
	model.Empty
}

// LoginRsp contains the URL a client should open to start authentication.
type LoginRsp struct {
	RedirectURL string `json:"redirect_url"`
}

func (Login) Design() {
	Route("/auth/login", func() {
		List(func() {
			Filename("login")
			Public()
			Service(true)
			Result[*LoginRsp]()
		})
	})
}
