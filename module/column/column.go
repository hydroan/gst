package column

import (
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/service"
)

var _ gst.Module[*empty, *empty, rsp] = (*mod)(nil)

type empty struct {
	model.Empty
}

type rsp = map[string][]string

type srv struct {
	service.Base[*empty, *empty, rsp]
}

type mod struct{}

func (*mod) Service() gst.Service[*empty, *empty, rsp] {
	return &srv{}
}
func (*mod) Pub() bool     { return false }
func (*mod) Route() string { return "column" }
func (*mod) Param() string { return "id" }
