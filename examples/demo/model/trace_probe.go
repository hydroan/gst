package model

import (
	"context"

	"github.com/hydroan/gst/database"
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// TraceProbe exercises standard CRUD context propagation through service,
// database, GORM, and model hooks.
type TraceProbe struct {
	Name string `json:"name" query:"name" gorm:"size:191;index"`
	Note string `json:"note,omitempty" query:"note" gorm:"size:1024"`

	Hook      string `json:"hook,omitempty" query:"hook" gorm:"size:64"`
	HookCount int64  `json:"hook_count,omitempty" gorm:"-"`

	model.Base
}

func (TraceProbe) GetTableName() string { return "demo_trace_probes" }
func (TraceProbe) Purge() bool          { return true }

func (TraceProbe) Design() {
	Migrate()
	Endpoint("trace-probes")
	Param("trace_probe")

	Create(func() {
		Filename("trace_probe")
		Service()
	})
	Delete(func() {
		Filename("trace_probe")
		Service()
	})
	Update(func() {
		Filename("trace_probe")
		Service()
	})
	Patch(func() {
		Filename("trace_probe")
		Service()
	})
	List(func() {
		Filename("trace_probe")
		Service()
	})
	Get(func() {
		Filename("trace_probe")
		Service()
	})
}

func (t *TraceProbe) CreateBefore(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.PHASE_CREATE_BEFORE)
}

func (t *TraceProbe) CreateAfter(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.PHASE_CREATE_AFTER)
}

func (t *TraceProbe) DeleteBefore(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.PHASE_DELETE_BEFORE)
}

func (t *TraceProbe) DeleteAfter(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.PHASE_DELETE_AFTER)
}

func (t *TraceProbe) UpdateBefore(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.PHASE_UPDATE_BEFORE)
}

func (t *TraceProbe) UpdateAfter(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.PHASE_UPDATE_AFTER)
}

func (t *TraceProbe) ListBefore(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.PHASE_LIST_BEFORE)
}

func (t *TraceProbe) ListAfter(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.PHASE_LIST_AFTER)
}

func (t *TraceProbe) GetBefore(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.PHASE_GET_BEFORE)
}

func (t *TraceProbe) GetAfter(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.PHASE_GET_AFTER)
}

func (t *TraceProbe) traceModelHook(ctx context.Context, phase consts.Phase) error {
	var total int64
	err := database.Database[*TraceProbe](ctx).Count(&total)
	if t != nil {
		t.Hook = phase.MethodName()
		t.HookCount = total
	}

	fields := traceProbeLogFields(t, phase, total)
	if logger.Database != nil {
		log := logger.Database.WithContext(ctx, phase)
		if err != nil {
			log.Errorz("trace probe model hook", append(fields, zap.Error(err))...)
		} else {
			log.Infoz("trace probe model hook", fields...)
		}
	}
	return err
}

func traceProbeLogFields(t *TraceProbe, phase consts.Phase, total int64) []zap.Field {
	fields := []zap.Field{
		zap.String("component", "model_hook"),
		zap.String("hook", phase.MethodName()),
		zap.Int64("total", total),
	}
	if t != nil {
		fields = append(
			fields,
			zap.String("probe_id", t.GetID()),
			zap.String("probe_name", t.Name),
		)
	}
	return fields
}
