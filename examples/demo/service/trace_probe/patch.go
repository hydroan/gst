package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

type Patcher struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *Patcher) PatchBefore(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_PATCH_BEFORE, probe, 0)
}

func (t *Patcher) PatchAfter(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_PATCH_AFTER, probe, 0)
}
