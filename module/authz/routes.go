package authz

import (
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/model"
	modelauthz "github.com/hydroan/gst/internal/modulemodel/authz"
	serviceauthz "github.com/hydroan/gst/internal/moduleservice/authz"
)

var _ gst.Module[*Routes, *model.Empty, *RoutesRsp] = (*RoutesModule)(nil)

type (
	Route        = modelauthz.Route
	Routes       = modelauthz.Routes
	RoutesRsp    = modelauthz.RoutesRsp
	RoutesModule struct{}
)

func (*RoutesModule) Service() gst.Service[*Routes, *model.Empty, *RoutesRsp] {
	return &serviceauthz.RoutesService{}
}
func (*RoutesModule) Route() string { return "authz/routes" }
func (*RoutesModule) Public() bool  { return false }
func (*RoutesModule) Param() string { return "id" }
