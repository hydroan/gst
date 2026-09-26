package model

import (
	"context"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/model"
	"go.uber.org/zap"
)

// TraceProbe exercises standard CRUD context propagation through service,
// database, GORM, and model hooks.
type TraceProbe struct {
	Name string `json:"name" query:"name" gorm:"size:191"`
	Note string `json:"note,omitempty" query:"note" gorm:"size:1024"`

	Hook      string `json:"hook,omitempty" query:"hook" gorm:"size:64"`
	HookCount int    `json:"hook_count,omitempty" gorm:"-"`

	model.Base
}

func (TraceProbe) TableName() string { return "demo_trace_probes" }
func (TraceProbe) Purge() bool       { return true }

func (TraceProbe) Design() {
	Migrate()
	Endpoint("trace-probes")
	Param("trace_probe")

	Create(func() {
		Service()
	})
	Delete(func() {
		Service()
	})
	Update(func() {
		Service()
	})
	Patch(func() {
		Service()
	})
	List(func() {
		Service()
	})
	Get(func() {
		Service()
	})
}

// Indexes declares the name lookup path the trace assertions filter by.
func (TraceProbe) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"Name"}},
	}
}

func (t *TraceProbe) CreateBefore(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.CreateBefore)
}

func (t *TraceProbe) CreateAfter(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.CreateAfter)
}

func (t *TraceProbe) DeleteBefore(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.DeleteBefore)
}

func (t *TraceProbe) DeleteAfter(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.DeleteAfter)
}

func (t *TraceProbe) UpdateBefore(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.UpdateBefore)
}

func (t *TraceProbe) UpdateAfter(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.UpdateAfter)
}

func (t *TraceProbe) ListBefore(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.ListBefore)
}

func (t *TraceProbe) ListAfter(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.ListAfter)
}

func (t *TraceProbe) GetBefore(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.GetBefore)
}

func (t *TraceProbe) GetAfter(ctx context.Context) error {
	return t.traceModelHook(ctx, consts.GetAfter)
}

func (t *TraceProbe) traceModelHook(ctx context.Context, phase consts.Phase) error {
	var total int
	err := database.Database[*TraceProbe](ctx).Count(&total)
	if t != nil {
		t.Hook = phase.Name()
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

func traceProbeLogFields(t *TraceProbe, phase consts.Phase, total int) []zap.Field {
	fields := []zap.Field{
		zap.String("component", "model_hook"),
		zap.String("hook", phase.Name()),
		zap.Int("total", total),
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
