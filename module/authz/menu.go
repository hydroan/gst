package authz

import (
	"github.com/hydroan/gst/internal/gst"
	modelauthz "github.com/hydroan/gst/internal/modulemodel/authz"
	serviceauthz "github.com/hydroan/gst/internal/moduleservice/authz"
)

var _ gst.Module[*Menu, *Menu, *Menu] = (*MenuModule)(nil)

type (
	Menu       = modelauthz.Menu
	MenuModule struct{}
)

func (*MenuModule) Service() gst.Service[*Menu, *Menu, *Menu] {
	return &serviceauthz.MenuService{}
}
func (*MenuModule) Route() string { return "authz/menus" }
func (*MenuModule) Public() bool  { return false }
func (*MenuModule) Param() string { return "id" }
