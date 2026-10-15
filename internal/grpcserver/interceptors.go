package grpcserver

import (
	"context"
	"sync"

	"github.com/cockroachdb/errors"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/realip"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/requestctx"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

// serverMetrics counts and times every call in the standard grpc_server_*
// metrics, on the default registry the metrics endpoint serves.
// registerServerMetrics puts them there once, on the first server Run
// starts, so a process without a gRPC service exposes none. The names are
// the library's own, unprefixed unlike the framework's HTTP metrics: they
// are what the gRPC dashboards in circulation query.
var (
	serverMetrics         = grpcprom.NewServerMetrics(grpcprom.WithServerHandlingTimeHistogram())
	registerServerMetrics = sync.OnceValue(func() error {
		return errors.Wrap(prometheus.Register(serverMetrics), "register the grpc server metrics")
	})
)

// chains returns the chains the server runs every call through, in the
// order the HTTP listener runs their counterparts: the client address
// first, which realip resolves from the forwarding headers of a peer
// server.trusted_proxies names (see requestctx.TrustedProxies) for the
// request scope to read (see enterCall); the request scope next, so every
// call downstream carries a trace id and leaves an access-log entry; the
// metrics then, which count what the recovery inside them
// turns a panic into; recovery, which covers the handler and everything
// after it; then the interceptors the project registered (see Use and
// UseAuth), closest to the handler, the way the HTTP listener mounts the
// project's middleware behind its own. The stream chain runs the same
// stages on a stream, the rpcs of the Stream actions and the health and
// reflection services' among them: the scope and the project's
// interceptors run once ahead of the first message (see requestScopeStream
// and streamOf); innermost, the stream gets the context that ends when the
// server begins to stop, stopping, and the stop's answer (see
// streamShutdown).
//
// With OpenTelemetry enabled the server also carries otelgrpc's stats
// handler, the counterpart of the HTTP listener's tracing middleware: it
// opens the server span of every call, unary or stream, at the transport
// layer ahead of the chains, from the trace context the call's metadata
// carries through the propagators otel.Init installed, names it by the full
// method, records the rpc attributes of the semantic conventions and the
// status the call ended with, and ends it after the response went out. The
// request scope reads the span for the call's trace id (see requestScope).
// The handler records no message events: the HTTP span carries no body
// events either.
func chains(stopping context.Context) ([]grpc.ServerOption, error) {
	proxies, err := requestctx.TrustedProxies(config.App.Server.TrustedProxies)
	if err != nil {
		return nil, err
	}
	// The same networks are the peers believed and the proxies skipped
	// along X-Forwarded-For, the way gin reads the list for the HTTP
	// listener, and the headers are the ones gin reads by default.
	forwarded := []realip.Option{
		realip.WithTrustedPeers(proxies),
		realip.WithTrustedProxies(proxies),
		realip.WithHeaders([]string{realip.XForwardedFor, realip.XRealIp}),
	}
	onPanic := recovery.WithRecoveryHandlerContext(recovered)
	unary := []grpc.UnaryServerInterceptor{realip.UnaryServerInterceptorOpts(forwarded...), requestScope, serverMetrics.UnaryServerInterceptor(), recovery.UnaryServerInterceptor(onPanic)}
	unary = append(unary, projectUnaryInterceptors()...)
	stream := []grpc.StreamServerInterceptor{realip.StreamServerInterceptorOpts(forwarded...), requestScopeStream, serverMetrics.StreamServerInterceptor(), recovery.StreamServerInterceptor(onPanic)}
	stream = append(stream, projectStreamInterceptors()...)
	stream = append(stream, streamShutdown(stopping))
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(unary...),
		grpc.ChainStreamInterceptor(stream...),
	}
	if gstotel.IsEnabled() {
		opts = append(opts, grpc.StatsHandler(otelgrpc.NewServerHandler()))
	}
	return opts, nil
}
