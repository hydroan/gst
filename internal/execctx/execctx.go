// Package execctx carries the identity of the unit of work a context belongs
// to. Every part of the framework that annotates its output with that
// identity — statement comments, the SQL log, the business log — reads it
// from here, whatever kind of work is running.
//
// The identity is deliberately kept apart from requestctx: an HTTP request is
// one kind of execution, and requestctx describes the request-specific facts
// of that kind — route, method, user, parameters. The two packages are peers
// and neither imports the other: the request middleware stamps the identity,
// NewServiceContext attaches the request metadata, and each consumer reads
// whichever it needs.
package execctx

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// Identity is what a context says about the unit of work it belongs to.
type Identity struct {
	// TraceID keys the trail the work leaves across logs, statement comments
	// and spans. The request middleware stamps it for a request; when nothing
	// stamped one it is borrowed from the span open on the context.
	TraceID string
	// Cronjob names the cron job whose round is running, and is "" outside a
	// round. The cron runner stamps it.
	Cronjob string
}

// identityKey keys the stamped Identity on a context. One key carries the
// whole identity, so resolving it costs a single context lookup on every
// path, the request path included.
type identityKey struct{}

// WithTraceID returns a context stamped with the trace id of the execution it
// belongs to, replacing any identity stamped before.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, identityKey{}, Identity{TraceID: traceID})
}

// WithCronjob returns a context stamped as one round of the named cron job,
// keyed by the round's trace id, replacing any identity stamped before.
func WithCronjob(ctx context.Context, name, traceID string) context.Context {
	return context.WithValue(ctx, identityKey{}, Identity{TraceID: traceID, Cronjob: name})
}

// FromContext resolves the identity of ctx: the stamped identity when there is
// one, else a trace id borrowed from the span open on ctx, else the zero
// Identity. A stamped identity wins over a span, so the id the middleware
// published to the caller stays the one every annotation carries.
func FromContext(ctx context.Context) Identity {
	if ctx == nil {
		return Identity{}
	}
	if id, ok := ctx.Value(identityKey{}).(Identity); ok {
		return id
	}
	if sc := trace.SpanFromContext(ctx).SpanContext(); sc.HasTraceID() {
		return Identity{TraceID: sc.TraceID().String()}
	}
	return Identity{}
}
