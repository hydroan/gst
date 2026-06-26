package authz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	serviceauthz "github.com/hydroan/gst/internal/service/authz"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*Menu, *Menu, *Menu] = (*MenuModule)(nil)

type (
	Menu       = modelauthz.Menu
	MenuModule struct{}
)

func (*MenuModule) Service() types.Service[*Menu, *Menu, *Menu] {
	return &serviceauthz.MenuService{}
}
func (*MenuModule) Route() string { return "authz/menus" }
func (*MenuModule) Pub() bool     { return false }
func (*MenuModule) Param() string { return "id" }
