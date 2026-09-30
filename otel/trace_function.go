package otel

import (
	"context"

	"go.opentelemetry.io/otel/codes"
)

// TraceFunction runs fn inside a span named functionName, ended when fn
// returns: an error fn returns is recorded on the span and returned, and a
// run without one leaves the span with the Ok status.
func TraceFunction(ctx context.Context, functionName string, fn func(context.Context) error) error {
	ctx, span := StartSpan(ctx, functionName)
	defer span.End()

	if err := fn(ctx); err != nil {
		RecordError(span, err)
		return err
	}

	span.SetStatus(codes.Ok, "")
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

	span.SetStatus(codes.Ok, "")
	return result, nil
}
