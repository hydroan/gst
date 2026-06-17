package authz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	serviceauthz "github.com/hydroan/gst/internal/service/authz"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*API, *API, APIRsp] = (*APIModule)(nil)

type (
	API       = modelauthz.API
	APIRsp    = modelauthz.APIRsp
	APIModule struct{}
)

func (*APIModule) Service() types.Service[*API, *API, APIRsp] {
	return &serviceauthz.APIService{}
}
func (*APIModule) Route() string { return "/apis" }
func (*APIModule) Pub() bool     { return false }
func (*APIModule) Param() string { return "id" }
