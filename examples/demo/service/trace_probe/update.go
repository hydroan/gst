package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/service"
)

type Updater struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *Updater) UpdateBefore(ctx *gst.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_UPDATE_BEFORE, probe, 0)
}

func (t *Updater) UpdateAfter(ctx *gst.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_UPDATE_AFTER, probe, 0)
}
