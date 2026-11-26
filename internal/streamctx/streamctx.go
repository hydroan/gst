// Package streamctx gives a stream a context that ends when the server it is
// served on begins to shut down.
//
// http.Server.Shutdown waits for every active request and cancels none of
// their contexts, so a stream watching only its request would hold the
// shutdown for as long as its client stays. The server makes its shutdown
// signal the base of every request context through WithServerShutdown, and a
// stream runs on the context New derives from its request, which ends with
// the request or with the shutdown, whichever comes first.
//
// It also holds what the streams of every kind keep alike, the pause between
// their keep-alives (see DefaultHeartbeatInterval).
package streamctx

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
)

// DefaultHeartbeatInterval is the pause a stream keeps between the
// keep-alives it sends its client while nothing else travels: the comment
// frames of a Server-Sent Events stream, the pings of a WebSocket
// connection. It stays well below the idle timeouts commonly configured on
// reverse proxies (nginx defaults to 60s), so a quiet stream is never
// mistaken for a dead one.
const DefaultHeartbeatInterval = 15 * time.Second

// ErrServerShutdown is the cause a stream's context ends with when the server
// the request arrived on begins to shut down, as opposed to the client going
// away.
var ErrServerShutdown = errors.New("streamctx: the server is shutting down")

type (
	// shutdownKey carries the shutdown signal of the server a request arrived
	// on.
	shutdownKey struct{}
	// streamKey marks a context New derived.
	streamKey struct{}
)

// WithServerShutdown returns ctx carrying shutdown, a context the server ends
// the moment it begins to shut down. The server makes it the base of every
// request context, and New reads it back. Only the signal travels: request
// contexts do not end with it, so ordinary requests still run to completion
// while the server drains.
func WithServerShutdown(ctx, shutdown context.Context) context.Context {
	return context.WithValue(ctx, shutdownKey{}, shutdown)
}

// New returns a context that ends with ctx or with the shutdown of the server
// the request arrived on, whichever comes first; the shutdown ends it with
// ErrServerShutdown as its cause. http.Server.Shutdown waits for every active
// request and cancels none of their contexts, so a stream watching only its
// request would hold the shutdown for as long as its client stays. stop
// releases the watch once the stream is over. A ctx that carries no server
// signal, or that New already derived, comes back as is.
func New(ctx context.Context) (stream context.Context, stop func()) {
	shutdown, ok := ctx.Value(shutdownKey{}).(context.Context)
	if !ok || ctx.Value(streamKey{}) != nil {
		return ctx, func() {}
	}
	stream, cancel := context.WithCancelCause(context.WithValue(ctx, streamKey{}, struct{}{}))
	stopWatch := context.AfterFunc(shutdown, func() { cancel(ErrServerShutdown) })
	return stream, func() {
		stopWatch()
		cancel(nil)
	}
}
