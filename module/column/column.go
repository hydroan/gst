package column

import (
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
)

var _ types.Module[*empty, *empty, rsp] = (*mod)(nil)

type empty struct {
	modelregistry.Empty
}

type rsp = map[string][]string

type srv struct {
	serviceregistry.Base[*empty, *empty, rsp]
}

type mod struct{}

func (*mod) Service() types.Service[*empty, *empty, rsp] {
	return &srv{}
}
func (*mod) Pub() bool     { return false }
func (*mod) Route() string { return "column" }
func (*mod) Param() string { return "id" }
