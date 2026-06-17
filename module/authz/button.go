package authz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	serviceauthz "github.com/hydroan/gst/internal/service/authz"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*Button, *Button, *Button] = (*ButtonModule)(nil)

type (
	Button       = modelauthz.Button
	ButtonModule struct{}
)

func (*ButtonModule) Service() types.Service[*Button, *Button, *Button] {
	return &serviceauthz.ButtonService{}
}
func (*ButtonModule) Route() string { return "buttons" }
func (*ButtonModule) Pub() bool     { return false }
func (*ButtonModule) Param() string { return "id" }
