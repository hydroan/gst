package traceprobe

import (
	"demo/model"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// TraceProbe service
//
//	curl -s -i -c ./cookies.txt \
//	  -X POST http://localhost:8090/api/login \
//	  -H 'Content-Type: application/json' \
//	  -d '{"username":"root","password":"toor"}'
//
//	curl -s -i -b ./cookies.txt \
//	  -X POST http://localhost:8090/api/trace-probes \
//	  -H 'Content-Type: application/json' \
//	  -d '{"name":"trace-probe-codex","note":"standard-crud-context"}'
//
//	curl -s -i -b ./demo-cookies.txt \
//	  'http://localhost:8090/api/trace-probes?name=trace-probe-codex'
//
//	curl -s -i -b ./cookies.txt \
//	  http://localhost:8090/api/trace-probes/019efee7-76e5-7520-a405-9d4c7bead437
type TraceProbe struct {
	service.Base[*model.TraceProbe, *model.TraceProbe, *model.TraceProbe]
}

func (t *TraceProbe) CreateBefore(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_CREATE_BEFORE, probe, 0)
}

func (t *TraceProbe) CreateAfter(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_CREATE_AFTER, probe, 0)
}

func (t *TraceProbe) DeleteBefore(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_DELETE_BEFORE, probe, 0)
}

func (t *TraceProbe) DeleteAfter(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_DELETE_AFTER, probe, 0)
}

func (t *TraceProbe) UpdateBefore(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_UPDATE_BEFORE, probe, 0)
}

func (t *TraceProbe) UpdateAfter(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_UPDATE_AFTER, probe, 0)
}

func (t *TraceProbe) PatchBefore(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_PATCH_BEFORE, probe, 0)
}

func (t *TraceProbe) PatchAfter(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_PATCH_AFTER, probe, 0)
}

func (t *TraceProbe) ListBefore(ctx *types.ServiceContext, probes *[]*model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_LIST_BEFORE, nil, traceProbeListLen(probes))
}

func (t *TraceProbe) ListAfter(ctx *types.ServiceContext, probes *[]*model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_LIST_AFTER, nil, traceProbeListLen(probes))
}

func (t *TraceProbe) GetBefore(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_GET_BEFORE, probe, 0)
}

func (t *TraceProbe) GetAfter(ctx *types.ServiceContext, probe *model.TraceProbe) error {
	return t.traceServiceHook(ctx, consts.PHASE_GET_AFTER, probe, 0)
}

func (t *TraceProbe) traceServiceHook(ctx *types.ServiceContext, phase consts.Phase, probe *model.TraceProbe, itemCount int) error {
	var total int64
	err := database.Database[*model.TraceProbe](ctx).Count(&total)

	fields := traceProbeServiceFields(probe, phase, total, itemCount)
	if t.Logger != nil {
		log := t.WithContext(ctx, phase)
		if err != nil {
			log.Errorz("trace probe service hook", append(fields, zap.Error(err))...)
		} else {
			log.Infoz("trace probe service hook", fields...)
		}
	}
	return err
}

func traceProbeServiceFields(probe *model.TraceProbe, phase consts.Phase, total int64, itemCount int) []zap.Field {
	fields := []zap.Field{
		zap.String("component", "service_hook"),
		zap.String("hook", phase.MethodName()),
		zap.Int64("total", total),
		zap.Int("item_count", itemCount),
	}
	if probe != nil {
		fields = append(
			fields,
			zap.String("probe_id", probe.GetID()),
			zap.String("probe_name", probe.Name),
		)
	}
	return fields
}

func traceProbeListLen(probes *[]*model.TraceProbe) int {
	if probes == nil {
		return 0
	}
	return len(*probes)
}
