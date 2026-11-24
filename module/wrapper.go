package module

import (
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/service"
)

var _ gst.Module[*model.Empty, *model.Empty, *model.Empty] = &Wrapper[*model.Empty, *model.Empty, *model.Empty]{}

type Wrapper[M gst.Model, REQ gst.Request, RSP gst.Response] struct {
	route string
	param string
	pub   bool
	svc   gst.Service[M, REQ, RSP]
}

func (w *Wrapper[M, REQ, RSP]) Service() gst.Service[M, REQ, RSP] {
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

func NewWrapper[M gst.Model, REQ gst.Request, RSP gst.Response](route string, param string, pub bool, svc ...gst.Service[M, REQ, RSP]) gst.Module[M, REQ, RSP] {
	if len(param) == 0 {
		param = apiaction.ParamID
	}

	var serviceImpl gst.Service[M, REQ, RSP]
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
