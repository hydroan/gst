package authz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/internal/modelregistry"
	serviceauthz "github.com/hydroan/gst/internal/service/authz"
	"github.com/hydroan/gst/internal/types"
)

var _ types.Module[*Routes, *modelregistry.Empty, *RoutesRsp] = (*RoutesModule)(nil)

type (
	Route        = modelauthz.Route
	Routes       = modelauthz.Routes
	RoutesRsp    = modelauthz.RoutesRsp
	RoutesModule struct{}
)

func (*RoutesModule) Service() types.Service[*Routes, *modelregistry.Empty, *RoutesRsp] {
	return &serviceauthz.RoutesService{}
}
func (*RoutesModule) Route() string { return "authz/routes" }
func (*RoutesModule) Pub() bool     { return false }
func (*RoutesModule) Param() string { return "id" }
