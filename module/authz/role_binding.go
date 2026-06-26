package authz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*RoleBinding, *RoleBinding, *RoleBinding] = (*RoleBindingModule)(nil)

type (
	RoleBinding       = modelauthz.RoleBinding
	RoleBindingModule struct{}
)

func (*RoleBindingModule) Service() types.Service[*RoleBinding, *RoleBinding, *RoleBinding] {
	return service.Base[*RoleBinding, *RoleBinding, *RoleBinding]{}
}
func (*RoleBindingModule) Route() string { return "authz/role-bindings" }
func (*RoleBindingModule) Pub() bool     { return false }
func (*RoleBindingModule) Param() string { return "id" }
