package rbac

import (
	"context"
	"maps"
	"strings"

	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
)

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// traceRBAC starts a gst-owned RBAC span and returns a finish callback.
// The returned context must be passed to Casbin so adapter and database spans
// appear under the RBAC operation in the request trace.
func traceRBAC(ctx context.Context, operation string, fields map[string]any) (context.Context, func(error)) {
	ctx = contextOrBackground(ctx)
	if !gstotel.IsEnabled() {
		return ctx, func(error) {}
	}

	spanCtx, span := gstotel.StartSpan(ctx, gstotel.OperationSpanName("rbac", operation))
	recording := gstotel.IsSpanRecording(span)
	if recording {
		tags := map[string]any{
			"component":      "rbac",
			"rbac.operation": operation,
		}
		maps.Copy(tags, fields)
		gstotel.AddSpanTags(span, tags)
	}

	return spanCtx, func(err error) {
		defer span.End()
		if !recording {
			return
		}
		gstotel.AddSpanTags(span, map[string]any{
			"rbac.success": err == nil,
		})
		if err != nil {
			gstotel.RecordError(span, err)
		}
	}
}

// traceAuthorize starts the span one decision is made under, and returns a
// callback that records what was decided rather than only whether deciding
// worked.
//
// It is separate from traceRBAC because a decision is the one operation whose
// result is worth recording: the writes around it either happen or report why
// they did not, while an authorization that answers perfectly well can still be
// the answer somebody is trying to explain. A trace that says only "succeeded"
// cannot tell an allow from a denial, so it cannot answer the question the span
// exists for.
//
// Nothing is built before the sampler has been consulted. This runs once per
// request, and a span that will not be recorded must cost no allocation at all;
// assembling the attributes first and discarding them would put that cost on
// every request in a deployment that samples, or traces not at all.
func traceAuthorize(ctx context.Context, tenant string) func(types.Decision, error) {
	if !gstotel.IsEnabled() {
		return func(types.Decision, error) {}
	}

	_, span := gstotel.StartSpan(contextOrBackground(ctx), gstotel.OperationSpanName("rbac", "authorize"))
	if !gstotel.IsSpanRecording(span) {
		return func(types.Decision, error) { span.End() }
	}
	gstotel.AddSpanTags(span, map[string]any{
		"component":      "rbac",
		"rbac.operation": "authorize",
		"rbac.tenant":    tenant,
	})

	return func(decision types.Decision, err error) {
		defer span.End()

		tags := map[string]any{
			"rbac.success": err == nil,
			"rbac.allowed": decision.Allowed,
		}
		// Exactly one of the two is set, and which one is the outcome: a grant
		// names the rule kind that allowed it, a denial names the step it was
		// missing. Recording both keys would leave a reader guessing which
		// applied.
		if decision.Source != "" {
			tags["rbac.allowed_by"] = string(decision.Source)
		}
		if decision.Reason != "" {
			tags["rbac.denied_by"] = string(decision.Reason)
		}
		// The matched rule is the policy row, not the request path: it carries
		// the template that matched, which is what an operator revokes. A span
		// attribute is not an index key, so its cardinality costs nothing here
		// the way it would on a metric label.
		if len(decision.MatchedRule) > 0 {
			tags["rbac.matched_rule"] = strings.Join(decision.MatchedRule, ",")
		}
		gstotel.AddSpanTags(span, tags)

		if err != nil {
			gstotel.RecordError(span, err)
		}
	}
}

// rbacTraceFields keeps RBAC span attributes low-cardinality enough for tracing.
// Subject identifiers are intentionally excluded because they are identity data
// and would make Jaeger labels noisy for role-binding write paths.
func rbacTraceFields(tenant string, role string) map[string]any {
	fields := make(map[string]any, 2)
	if tenant = strings.TrimSpace(tenant); tenant != "" {
		fields["rbac.tenant"] = tenant
	}
	if role = strings.TrimSpace(role); role != "" {
		fields["rbac.role"] = role
	}
	return fields
}
