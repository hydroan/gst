package module

import (
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
)

var _ types.Module[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty] = &Wrapper[*modelregistry.Empty, *modelregistry.Empty, *modelregistry.Empty]{}

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
	return &serviceregistry.Base[M, REQ, RSP]{}
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
