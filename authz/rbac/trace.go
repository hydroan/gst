package rbac

import (
	"context"
	"maps"
	"strings"

	gstotel "github.com/hydroan/gst/provider/otel"
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
