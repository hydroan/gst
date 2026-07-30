package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

type Getter struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *Getter) GetBefore(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_GET_BEFORE, probe, 0)
}

func (t *Getter) GetAfter(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_GET_AFTER, probe, 0)
}
