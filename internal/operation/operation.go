// Package operation begins the framework's own background operations — a
// round of a cron job, a tenure of a leader work — with the identity the rest
// of the operation is annotated with: a trace id, taken from a root span when
// tracing is on and generated otherwise, the way a request takes its own at
// the listener.
package operation

import (
	"context"

	"github.com/hydroan/gst/internal/util"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/trace"
)

// Begin returns the context the named operation of component runs on, its
// root span and its trace id. With tracing on the span is started under
// gstotel.OperationSpanName(component, name) — the parent of every span the
// operation's work opens — and the id is the span's; with tracing off the
// span is nil and the id is generated. The caller ends the span.
func Begin(ctx context.Context, component, name string) (context.Context, trace.Span, string) {
	if !gstotel.IsEnabled() {
		return ctx, nil, util.TraceID()
	}
	ctx, span := gstotel.StartSpan(ctx, gstotel.OperationSpanName(component, name))
	return ctx, span, span.SpanContext().TraceID().String()
}
