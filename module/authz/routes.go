package authz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	serviceauthz "github.com/hydroan/gst/internal/service/authz"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*Routes, *Routes, RoutesRsp] = (*RoutesModule)(nil)

type (
	Route        = modelauthz.Route
	Routes       = modelauthz.Routes
	RoutesRsp    = modelauthz.RoutesRsp
	RoutesModule struct{}
)

func (*RoutesModule) Service() types.Service[*Routes, *Routes, RoutesRsp] {
	return &serviceauthz.RoutesService{}
}
func (*RoutesModule) Route() string { return "routes" }
func (*RoutesModule) Pub() bool     { return false }
func (*RoutesModule) Param() string { return "id" }
