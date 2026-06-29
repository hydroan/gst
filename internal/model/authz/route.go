package modelauthz

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Routes represents the list action model for registered backend routes.
type Routes struct {
	model.Empty
}

func (Routes) Design() {
	dsl.Route("authz/routes", func() {
		dsl.List(func() {
			dsl.Service()
			dsl.Flatten()
			dsl.Filename("routes.go")
			dsl.Result[RoutesRsp]()
		})
	})
}

// Route is a registered backend route that can be bound to a menu.
type Route struct {
	Path    string   `json:"path" schema:"path"`
	Methods []string `json:"methods" schema:"methods"`
}

// RoutesRsp is the response returned by GET /api/authz/routes.
type RoutesRsp struct {
	Items []Route `json:"items"`
}
