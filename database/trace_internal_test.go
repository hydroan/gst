package database

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/stretchr/testify/require"
)

// traceSample is a model for the tracing helpers alone: never registered and
// never written, it only names the spans and overrides one hook so that the
// hook span exists.
type traceSample struct {
	modelregistry.Base
}

func (*traceSample) TableName() string { return "trace_samples" }

func (*traceSample) CreateBefore(context.Context) error { return nil }

// TestOperationSpanAttributesFitTheCapacityInTheWorstCase pins the two
// batches an operation span carries — the start batch of a batched
// operation, the outcome batch of a failed one — so an attribute added
// without bumping its capacity fails here instead of regrowing the slice on
// every operation.
func TestOperationSpanAttributesFitTheCapacityInTheWorstCase(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)

	db := &database[*traceSample]{ins: DB(), ctx: context.Background()}
	done, _ := db.traceAs("TraceSample", consts.PHASE_CREATE, 3)
	done(errors.New("sample failure"))

	span := oteltest.EndedNamed(t, recorder, "database.TraceSample.Create")
	require.Len(t, span.Attributes(), operationStartAttrCap+operationOutcomeAttrCap,
		"the worst case must fill both batches exactly: a new attribute bumps operationStartAttrCap or operationOutcomeAttrCap")
}

// TestOperationLogFieldsFitTheCapacityInTheWorstCase pins operationLogFieldCap
// to the entry of a batched dry run that failed — every optional field
// present — so a field added without bumping the capacity fails here instead
// of regrowing the slice on every operation.
func TestOperationLogFieldsFitTheCapacityInTheWorstCase(t *testing.T) {
	fields := operationLogFields("TraceSample", 3, time.Millisecond, true, errors.New("sample failure"))

	require.Len(t, fields, operationLogFieldCap, "the worst case must fill the capacity exactly: a new field bumps operationLogFieldCap")
	require.Equal(t, operationLogFieldCap, cap(fields), "the slice must not have regrown")
}

// TestHookSpanAttributesFitTheCapacityInTheWorstCase pins hookOutcomeAttrCap
// to the outcome batch of a hook that failed, on top of the three attributes
// the hook span starts with.
func TestHookSpanAttributesFitTheCapacityInTheWorstCase(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)

	ctx, parent := gstotel.StartSpan(context.Background(), "sample.parent")
	err := traceModelHook[*traceSample](ctx, consts.PHASE_CREATE_BEFORE, parent, func(context.Context) error {
		return errors.New("sample hook failure")
	})
	parent.End()
	require.Error(t, err)

	span := oteltest.EndedNamed(t, recorder, "model.TraceSample.CreateBefore")
	// component, model.model and model.phase are set as the span starts; the
	// outcome batch adds the rest.
	const startAttrs = 3
	require.Len(t, span.Attributes(), startAttrs+hookOutcomeAttrCap,
		"the worst case must fill the outcome batch exactly: a new attribute bumps hookOutcomeAttrCap")
}
