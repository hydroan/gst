package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/service"
)

type Getter struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *Getter) GetBefore(ctx *gst.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.GetBefore, probe, 0)
}

func (t *Getter) GetAfter(ctx *gst.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.GetAfter, probe, 0)
}
