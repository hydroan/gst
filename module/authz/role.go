package authz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	serviceauthz "github.com/hydroan/gst/internal/service/authz"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*Role, *Role, *Role] = (*RoleModule)(nil)

type (
	Role       = modelauthz.Role
	RoleModule struct{}
)

func (*RoleModule) Service() types.Service[*Role, *Role, *Role] {
	return &serviceauthz.RoleService{}
}
func (*RoleModule) Route() string { return "authz/roles" }
func (*RoleModule) Pub() bool     { return false }
func (*RoleModule) Param() string { return "id" }
