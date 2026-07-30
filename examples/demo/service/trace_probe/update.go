package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

type Updater struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *Updater) UpdateBefore(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_UPDATE_BEFORE, probe, 0)
}

func (t *Updater) UpdateAfter(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_UPDATE_AFTER, probe, 0)
}
