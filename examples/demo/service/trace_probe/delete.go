package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

type Deleter struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *Deleter) DeleteBefore(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_DELETE_BEFORE, probe, 0)
}

func (t *Deleter) DeleteAfter(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_DELETE_AFTER, probe, 0)
}
