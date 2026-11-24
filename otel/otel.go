// Package otel traces a project's own work beside the spans the framework
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
// A project opens a span of its own with StartSpan, under a name
// OperationSpanName builds, and records a failure on it with RecordError;
// TraceFunction and TraceFunctionWithResult wrap a function in a span,
// recording the error it returns.
//
// The implementation lives in internal/otel; this package forwards the part a
// project uses.
package otel

import (
	"context"

	gstotel "github.com/hydroan/gst/internal/otel"
	"go.opentelemetry.io/otel/trace"
)

// OperationSpanName returns the name of a span in the form the framework
// names its own: the component in snake_case and the operation in
// UpperCamelCase, joined by a dot, so OperationSpanName("sample",
// "import_rows") is "sample.ImportRows".
func OperationSpanName(component string, operation string) string {
	return gstotel.OperationSpanName(component, operation)
}

// StartSpan starts a span named name under the span ctx carries and returns
// it with the context that carries it. The caller owns the span and ends it
// when the traced work finishes. With tracing off nothing is started: ctx
// comes back as it is, with the span it already carries.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return gstotel.StartSpan(ctx, name, opts...)
}

// RecordError records err on span as an exception event and marks the span
// failed; a nil err, or a span that does not record, is left alone.
//
// The stack trace recorded with the event is the one err carries from where
// it was created, the deepest in its unwrap chain, as errors from
// github.com/cockroachdb/errors do, so the span points at where the error
// happened. An error carrying none is recorded with the stack of the
// RecordError call.
func RecordError(span trace.Span, err error) { gstotel.RecordError(span, err) }

// TraceFunction runs fn inside a span named functionName, ended when fn
// returns: an error fn returns is recorded on the span, which marks it
// failed, and returned; a run without one leaves the span's status unset,
// the way instrumentation does.
func TraceFunction(ctx context.Context, functionName string, fn func(context.Context) error) error {
	return gstotel.TraceFunction(ctx, functionName, fn)
}

// TraceFunctionWithResult is TraceFunction for a function returning a result
// along with its error; the result is returned either way.
func TraceFunctionWithResult[T any](ctx context.Context, functionName string, fn func(context.Context) (T, error)) (T, error) {
	return gstotel.TraceFunctionWithResult(ctx, functionName, fn)
}
