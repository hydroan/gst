package otel

import (
	"context"
)

// TraceFunction runs fn inside a span named functionName, ended when fn
// returns: an error fn returns is recorded on the span, which marks it
// failed, and returned; a run without one leaves the span's status unset,
// the way instrumentation does.
func TraceFunction(ctx context.Context, functionName string, fn func(context.Context) error) error {
	ctx, span := StartSpan(ctx, functionName)
	defer span.End()

	if err := fn(ctx); err != nil {
		RecordError(span, err)
		return err
	}
	return nil
}

// TraceFunctionWithResult is TraceFunction for a function returning a result
// along with its error; the result is returned either way.
func TraceFunctionWithResult[T any](ctx context.Context, functionName string, fn func(context.Context) (T, error)) (T, error) {
	ctx, span := StartSpan(ctx, functionName)
	defer span.End()

	result, err := fn(ctx)
	if err != nil {
		RecordError(span, err)
		return result, err
	}
	return result, nil
}
