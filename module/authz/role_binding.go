package authz

import (
	"github.com/hydroan/gst/internal/gst"
	modelauthz "github.com/hydroan/gst/internal/modulemodel/authz"
	"github.com/hydroan/gst/internal/service"
)

var _ gst.Module[*RoleBinding, *RoleBinding, *RoleBinding] = (*RoleBindingModule)(nil)

type (
	RoleBinding       = modelauthz.RoleBinding
	RoleBindingModule struct{}
)

func (*RoleBindingModule) Service() gst.Service[*RoleBinding, *RoleBinding, *RoleBinding] {
	return service.Base[*RoleBinding, *RoleBinding, *RoleBinding]{}
}
func (*RoleBindingModule) Route() string { return "authz/role-bindings" }
func (*RoleBindingModule) Pub() bool     { return false }
func (*RoleBindingModule) Param() string { return "id" }
