package authz

import (
	"github.com/hydroan/gst/internal/gst"
	modelauthz "github.com/hydroan/gst/internal/modulemodel/authz"
	"github.com/hydroan/gst/internal/service"
)

var _ gst.Module[*Role, *Role, *Role] = (*RoleModule)(nil)

type (
	Role       = modelauthz.Role
	RoleModule struct{}
)

func (*RoleModule) Service() gst.Service[*Role, *Role, *Role] {
	return service.Base[*Role, *Role, *Role]{}
}
func (*RoleModule) Route() string { return "authz/roles" }
func (*RoleModule) Pub() bool     { return false }
func (*RoleModule) Param() string { return "id" }
