package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

type Creator struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *Creator) CreateBefore(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_CREATE_BEFORE, probe, 0)
}

func (t *Creator) CreateAfter(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_CREATE_AFTER, probe, 0)
}
