package grpcserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/util"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// traceIDKey is the metadata key the trace id travels under, in and out:
// the HTTP listener's X-Trace-ID header, in the lowercase gRPC metadata
// keeps its keys in.
const traceIDKey = "x-trace-id"

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

// requestScope gives a call what the HTTP listener's tracing and access-log
// middleware give a request. It stamps the call's trace id on the context
// as the identity of the execution — the caller's x-trace-id, or a
// generated one — and publishes it in the response header, which goes out
// with the status of a failed call as well; attaches the request metadata a
// ServiceContext built on the context answers for; and, once the handler
// returns, writes the call's entry to the access log with the fields the
// HTTP entry carries, the status being the code's name and, for a failed
// call, the status message beside it.
//
// The metadata is what the call itself says: the full method as route, path
// and request URI, POST as the method every gRPC call is on the wire, the
// address of the peer and whether it speaks TLS, the authority the call was
// addressed to and the user agent. The forwarding headers of a proxy in
// front are not read: the HTTP listener believes them from the peers
// server.trusted_proxies names alone, a judgement gin makes for it and this
// listener has no gin to make.
func requestScope(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	md, _ := metadata.FromIncomingContext(ctx)
	traceID := first(md, traceIDKey)
	if traceID == "" {
		traceID = util.SpanID()
	}
	ctx = execctx.WithTraceID(ctx, traceID)
	address, tls := peerOf(ctx)
	meta := requestctx.New(requestctx.Fields{
		Route:      info.FullMethod,
		Path:       info.FullMethod,
		RequestURI: info.FullMethod,
		Method:     http.MethodPost,
		ClientIP:   address,
		UserAgent:  first(md, "user-agent"),
		Host:       first(md, ":authority"),
		TLS:        tls,
	})
	ctx = requestctx.WithMetadata(ctx, meta)
	// SetHeader fails only on a context carrying no call, which the
	// server's own contexts never are.
	_ = grpc.SetHeader(ctx, metadata.Pairs(traceIDKey, traceID))

	rsp, err := handler(ctx, req)

	if logger.GRPC == nil {
		// A process that never initialized its loggers, which bootstrap
		// always does before Run; its other loggers drop entries too.
		return rsp, err
	}
	// accessLogFieldCap must stay >= the number of fields appended below,
	// so the slice is allocated once per call; re-check it when adding or
	// removing a field.
	const accessLogFieldCap = 12
	st := statusOf(err)
	fields := make([]zapcore.Field, 0, accessLogFieldCap)
	fields = append(
		fields,
		zap.String("status", st.Code().String()),
		zap.String(consts.CTX_METHOD, meta.Method()),
		zap.String(consts.CTX_USERNAME, meta.Username()),
		zap.String(consts.CTX_USER_ID, meta.UserID()),
		zap.String(consts.TRACE_ID, traceID),
		zap.String(consts.CTX_ROUTE, meta.Route()),
		zap.String(consts.CTX_PATH, meta.Path()),
		zap.String("ip", meta.ClientIP()),
		zap.String("user_agent", meta.UserAgent()),
		util.LogDuration(time.Since(start)),
	)
	if err != nil {
		fields = append(fields, zap.String("error", st.Message()))
	}
	logger.GRPC.Info(info.FullMethod, fields...)
	return rsp, err
}

// statusOf returns the status the server answers err with, the way grpc-go
// derives it: the status of a status error, Canceled or DeadlineExceeded for
// a context error, Unknown for any other error and OK for none.
func statusOf(err error) *status.Status {
	if err == nil {
		return status.New(codes.OK, "")
	}
	if st, ok := status.FromError(err); ok {
		return st
	}
	return status.FromContextError(err)
}

// first returns the first value md carries under key, "" when none.
func first(md metadata.MD, key string) string {
	if values := md.Get(key); len(values) > 0 {
		return values[0]
	}
	return ""
}

// peerOf returns the address of the peer the call came from, without its
// port the way gin's ClientIP reports an HTTP client, and whether the
// connection speaks TLS; "" and false for a call with no peer, which only a
// handler called outside a server sees.
func peerOf(ctx context.Context) (address string, tls bool) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return "", false
	}
	address = p.Addr.String()
	if host, _, err := net.SplitHostPort(address); err == nil {
		address = host
	}
	_, tls = p.AuthInfo.(credentials.TLSInfo)
	return address, tls
}

// recovered is the recovery handler of both chains. It logs the panic with
// the method, the trace id and the stack to the recovery log, the one the
// HTTP listener's panics go to, and answers codes.Internal with the message
// the HTTP envelope carries for the same case; what the panic was stays in
// the log, and the caller quotes back the trace id the response header
// carries.
func recovered(ctx context.Context, p any) error {
	if logger.Recovery != nil {
		method, _ := grpc.Method(ctx)
		logger.Recovery.Error(
			fmt.Sprintf("[recovery] panic recovered:\n%s\n%v\n%s", method, p, debug.Stack()),
			zap.String(consts.TRACE_ID, execctx.FromContext(ctx).TraceID),
		)
	}
	return status.Error(codes.Internal, "internal server error")
}
