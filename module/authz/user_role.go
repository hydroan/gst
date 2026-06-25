package authz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*UserRole, *UserRole, *UserRole] = (*UserRoleModule)(nil)

type (
	UserRole       = modelauthz.UserRole
	UserRoleModule struct{}
)

func (*UserRoleModule) Service() types.Service[*UserRole, *UserRole, *UserRole] {
	return service.Base[*UserRole, *UserRole, *UserRole]{}
}
func (*UserRoleModule) Route() string { return "authz/user-roles" }
func (*UserRoleModule) Pub() bool     { return false }
func (*UserRoleModule) Param() string { return "id" }
