package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/service"
)

type Lister struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *Lister) ListBefore(ctx *gst.ServiceContext, probes *[]*model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.ListBefore, nil, traceProbeListLen(probes))
}

func (t *Lister) ListAfter(ctx *gst.ServiceContext, probes *[]*model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.ListAfter, nil, traceProbeListLen(probes))
}
