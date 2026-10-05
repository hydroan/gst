package otel

import (
	"context"
	"strings"

	"github.com/hydroan/gst/internal/consts"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// traceIDHeaderPropagator extracts a parent span context from the trace id
// header the framework's own clients and logs carry, X-Trace-ID (with the
// span id of X-Span-ID when sent), for a request or call that carries no
// W3C trace context: the caller's trace id then becomes the server span's,
// so a caller quoting it finds the span. It is the last propagator of the
// composite Init installs, and it defers to what the ones before it found:
// a valid trace context wins over the header. Both listeners read it, the
// HTTP one through the request headers and the gRPC one through the call's
// metadata, which is what makes it a propagator rather than a header
// helper of one listener. It injects nothing: outbound requests carry the
// W3C headers the other propagators write, and the framework's client sets
// the header itself.
type traceIDHeaderPropagator struct{}

// defaultSpanID stands in for a caller that sent a trace id and no span id.
var defaultSpanID = trace.SpanID{0, 0, 0, 0, 0, 0, 0, 1}

func (traceIDHeaderPropagator) Inject(context.Context, propagation.TextMapCarrier) {}

func (traceIDHeaderPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	if trace.SpanContextFromContext(ctx).IsValid() {
		return ctx
	}
	traceID, err := trace.TraceIDFromHex(strings.TrimSpace(carrier.Get(consts.HEADER_TRACE_ID)))
	if err != nil {
		return ctx
	}
	spanID := defaultSpanID
	if value := strings.TrimSpace(carrier.Get(consts.HEADER_SPAN_ID)); value != "" {
		if parsed, err := trace.SpanIDFromHex(value); err == nil {
			spanID = parsed
		}
	}
	return trace.ContextWithRemoteSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	}))
}

func (traceIDHeaderPropagator) Fields() []string {
	return []string{consts.HEADER_TRACE_ID, consts.HEADER_SPAN_ID}
}
