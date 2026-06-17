package authz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*Permission, *Permission, *Permission] = (*PermissionModule)(nil)

type (
	Permission       = modelauthz.Permission
	PermissionModule struct{}
)

func (*PermissionModule) Service() types.Service[*Permission, *Permission, *Permission] {
	return service.Base[*Permission, *Permission, *Permission]{}
}
func (*PermissionModule) Route() string { return "authz/permissions" }
func (*PermissionModule) Pub() bool     { return false }
func (*PermissionModule) Param() string { return "id" }
