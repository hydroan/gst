package column

import (
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*empty, *empty, rsp] = (*mod)(nil)

type empty struct {
	model.Empty
}

type rsp = map[string][]string

type srv struct {
	service.Base[*empty, *empty, rsp]
}

type mod struct{}

func (*mod) Service() types.Service[*empty, *empty, rsp] {
	return &srv{}
}
func (*mod) Pub() bool     { return false }
func (*mod) Route() string { return "column" }
func (*mod) Param() string { return "id" }
