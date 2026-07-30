package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

type Lister struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *Lister) ListBefore(ctx *types.ServiceContext, probes *[]*model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_LIST_BEFORE, nil, traceProbeListLen(probes))
}

func (t *Lister) ListAfter(ctx *types.ServiceContext, probes *[]*model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.PHASE_LIST_AFTER, nil, traceProbeListLen(probes))
}
