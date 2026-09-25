package grpcserver

import (
	"sync"

	"github.com/cockroachdb/errors"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/prometheus/client_golang/prometheus"
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

// interceptors returns the chains the server runs every call through, in
// the order the HTTP listener runs their counterparts: the request scope
// first, so every call downstream carries a trace id and leaves an
// access-log entry; the metrics next, which then count what the recovery
// inside them turns a panic into; recovery last, closest to the handler.
// The stream chain carries no request scope: the framework serves no
// streaming rpc of its own, the health and reflection services being the
// only streams, so it counts and recovers them and nothing more.
func interceptors() []grpc.ServerOption {
	onPanic := recovery.WithRecoveryHandlerContext(recovered)
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(requestScope, serverMetrics.UnaryServerInterceptor(), recovery.UnaryServerInterceptor(onPanic)),
		grpc.ChainStreamInterceptor(serverMetrics.StreamServerInterceptor(), recovery.StreamServerInterceptor(onPanic)),
	}
}
