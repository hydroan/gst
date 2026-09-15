package rbac

import (
	"context"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// TestRBACTraceFieldsFitTheCapacityInTheWorstCase pins rbacTraceFieldCap to
// the fields rbacTraceFields returns when both are present, so a field added
// without bumping the capacity fails here instead of regrowing the slice on
// every traced write.
func TestRBACTraceFieldsFitTheCapacityInTheWorstCase(t *testing.T) {
	fields := rbacTraceFields("sample-tenant", "sample-role")

	require.Len(t, fields, rbacTraceFieldCap, "the worst case must fill the capacity exactly: a new field bumps rbacTraceFieldCap")
	require.Equal(t, rbacTraceFieldCap, cap(fields), "the slice must not have regrown")
}

// TestTraceRBACAttributesFitTheCapacityInTheWorstCase pins
// rbacOperationAttrCount to what traceRBAC records ahead of the operation's
// own fields: the span ends up with those, the fields, and the success flag
// the finish callback adds, and nothing else.
func TestTraceRBACAttributesFitTheCapacityInTheWorstCase(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)

	fields := rbacTraceFields("sample-tenant", "sample-role")
	_, finish := traceRBAC(context.Background(), "sample", fields)
	finish(nil)

	span := oteltest.EndedNamed(t, recorder, "rbac.Sample")
	// The finish callback adds rbac.success on top of the batch.
	const finishAttrs = 1
	require.Len(t, span.Attributes(), rbacOperationAttrCount+len(fields)+finishAttrs,
		"the operation attributes, the fields and the success flag: a new operation attribute bumps rbacOperationAttrCount")
}

// TestTraceAuthorizeAttributesFitTheCapacityInTheWorstCase pins
// authorizeOutcomeAttrCap to the outcome batch of a decision carrying every
// optional attribute — a grant source, a denial reason and a matched rule —
// on top of the three attributes the span starts with.
func TestTraceAuthorizeAttributesFitTheCapacityInTheWorstCase(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)

	finish := traceAuthorize(context.Background(), "sample-tenant")
	finish(types.Decision{
		Allowed:     true,
		Source:      consts.GrantSourceRole,
		Reason:      consts.DenyReason("sample-denial"),
		MatchedRule: []string{"sample-tenant", "/api/samples", "GET"},
	}, nil)

	span := oteltest.EndedNamed(t, recorder, "rbac.Authorize")
	// component, rbac.operation and rbac.tenant are set as the span starts;
	// the outcome batch adds the rest.
	const startAttrs = 3
	require.Len(t, span.Attributes(), startAttrs+authorizeOutcomeAttrCap,
		"the worst case must fill the outcome batch exactly: a new outcome attribute bumps authorizeOutcomeAttrCap")
}
