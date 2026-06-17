package module

import (
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*model.Empty, *model.Empty, *model.Empty] = &Wrapper[*model.Empty, *model.Empty, *model.Empty]{}

type Wrapper[M types.Model, REQ types.Request, RSP types.Response] struct {
	route string
	param string
	pub   bool
	svc   types.Service[M, REQ, RSP]
}

func (w *Wrapper[M, REQ, RSP]) Service() types.Service[M, REQ, RSP] {
	if w.svc != nil {
		return w.svc
	}
	return &service.Base[M, REQ, RSP]{}
}

func (w *Wrapper[M, REQ, RSP]) Route() string {
	return w.route
}

func (w *Wrapper[M, REQ, RSP]) Pub() bool {
	return w.pub
}

func (w *Wrapper[M, REQ, RSP]) Param() string {
	return w.param
}

func NewWrapper[M types.Model, REQ types.Request, RSP types.Response](route string, param string, pub bool, svc ...types.Service[M, REQ, RSP]) types.Module[M, REQ, RSP] {
	if len(param) == 0 {
		param = "id"
	}

	var serviceImpl types.Service[M, REQ, RSP]
	if len(svc) > 0 && svc[0] != nil {
		serviceImpl = svc[0]
	}

	return &Wrapper[M, REQ, RSP]{
		route: route,
		param: param,
		pub:   pub,
		svc:   serviceImpl,
	}
}
