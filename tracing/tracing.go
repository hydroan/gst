// Package tracing traces a project's own work beside the spans the framework
// records for every request. Both are exported over OTLP, to Jaeger or any
// other OTLP-compatible backend, Uptrace among them.
//
// Tracing is configured in the [otel] section; the framework starts it with
// the application and stops it on shutdown.
//
// Supported exporter types:
//   - http/protobuf: OTLP over HTTP using protobuf payloads.
//   - grpc: OTLP over gRPC.
//
// Full sampling configuration:
//
//	[otel]
//	enabled = true
//	service_name = demo
//	exporter_otlp_protocol = http/protobuf
//	exporter_otlp_traces_endpoint = http://localhost:4318/v1/traces
//	traces_sampler = parentbased_always_on
//
// Partial sampling configuration:
//
//	[otel]
//	enabled = true
//	service_name = demo
//	exporter_otlp_protocol = http/protobuf
//	exporter_otlp_traces_endpoint = http://localhost:4318/v1/traces
//	traces_sampler = parentbased_traceidratio
//	traces_sampler_arg = 0.1
//	bsp_max_queue_size = 2048
//	bsp_max_export_batch_size = 512
//	bsp_schedule_delay = 5s
//	bsp_export_timeout = 30s
//
// Use traces_sampler=parentbased_traceidratio with traces_sampler_arg between
// 0 and 1 to enable partial sampling while honoring upstream sampling
// decisions. For example, traces_sampler_arg=0.1 samples about 10% of root
// traces.
//
// A project opens a span of its own with StartSpan, under a name SpanName
// builds, and records a failure on it with RecordError; WithSpan and
// WithSpanResult run a function inside a span, recording the error it
// returns.
//
// The implementation lives in internal/tracing; this package forwards the
// part a project uses.
package tracing

import (
	"context"

	"github.com/hydroan/gst/internal/tracing"
	"go.opentelemetry.io/otel/trace"
)

// SpanName returns the name of a span in the form the framework names its
// own: the component in snake_case and the operation in UpperCamelCase,
// joined by a dot, so SpanName("sample", "import_rows") is
// "sample.ImportRows".
func SpanName(component string, operation string) string {
	return tracing.SpanName(component, operation)
}

// StartSpan starts a span named name under the span ctx carries and returns
// it with the context that carries it. The caller owns the span and ends it
// when the traced work finishes. With tracing off nothing is started: ctx
// comes back as it is, with the span it already carries.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return tracing.StartSpan(ctx, name, opts...)
}

// RecordError records err on span as an exception event and marks the span
// failed; a nil err, or a span that does not record, is left alone.
//
// The stack trace recorded with the event is the one err carries from where
// it was created, the deepest in its unwrap chain, as errors from
// github.com/cockroachdb/errors do, so the span points at where the error
// happened. An error carrying none is recorded with the stack of the
// RecordError call.
func RecordError(span trace.Span, err error) { tracing.RecordError(span, err) }

// WithSpan runs fn inside a span named name, ended when fn returns: an error
// fn returns is recorded on the span, which marks it failed, and returned; a
// run without one leaves the span's status unset, the way instrumentation
// does.
func WithSpan(ctx context.Context, name string, fn func(context.Context) error) error {
	return tracing.WithSpan(ctx, name, fn)
}

// WithSpanResult is WithSpan for a function returning a result along with
// its error; the result is returned either way.
func WithSpanResult[T any](ctx context.Context, name string, fn func(context.Context) (T, error)) (T, error) {
	return tracing.WithSpanResult(ctx, name, fn)
}
