package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/service"
)

type Deleter struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *Deleter) DeleteBefore(ctx *gst.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.DeleteBefore, probe, 0)
}

func (t *Deleter) DeleteAfter(ctx *gst.ServiceContext, probe *model.TraceProbe) error {
	return traceServiceHook(t.Logger, ctx, consts.DeleteAfter, probe, 0)
}
