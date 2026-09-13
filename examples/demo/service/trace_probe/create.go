package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *Creator) CreateBefore(ctx *gst.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_CREATE_BEFORE, probe, 0)
}

func (t *Creator) CreateAfter(ctx *gst.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_CREATE_AFTER, probe, 0)
}
