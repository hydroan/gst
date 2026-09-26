package grpcserver

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/util"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// The metadata keys the request scope reads, in the lowercase gRPC metadata
// keeps its keys in: traceIDKey carries the trace id in and out, the HTTP
// listener's X-Trace-ID header; userAgentKey and authorityKey are what the
// User-Agent header and the request's host arrive as.
const (
	traceIDKey   = "x-trace-id"
	userAgentKey = "user-agent"
	authorityKey = ":authority"
)

// accessLogFieldCap is the most fields an access-log entry carries, the ten
// every entry has plus the error of a failed call, so the slice is allocated
// once per call; a test holds the worst case to it, which is what keeps it
// honest when a field is added.
const accessLogFieldCap = 11

// requestScope gives a call what the HTTP listener's tracing and access-log
// middleware give a request. It stamps the call's trace id on the context
// as the identity of the execution — the server span's when tracing is on,
// the span being the root the call's inner spans hang off; the caller's
// x-trace-id otherwise, which with tracing on seeds the span's trace id the
// way the X-Trace-ID header does over HTTP; or a generated one — and
// publishes it in the response header, which goes out with the status of a
// failed call as well; attaches the request metadata a ServiceContext built
// on the context answers for, and keeps the call record an authentication
// interceptor later adds the caller to (see WithCaller); and, once the
// handler returns, writes the call's entry to the access log with the
// fields the HTTP entry carries, the caller as established by then, the
// status being the code's name and, for a failed call, the status message
// beside it.
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
	var traceID string
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		traceID = sc.TraceID().String()
		ctx = gstotel.ContextWithRequestRootSpan(ctx)
	} else if traceID = first(md, traceIDKey); traceID == "" {
		traceID = util.SpanID()
	}
	ctx = execctx.WithTraceID(ctx, traceID)
	address, tls := peerOf(ctx)
	c := &callRecord{method: methods[info.FullMethod], fields: requestctx.Fields{
		Route:      info.FullMethod,
		Path:       info.FullMethod,
		RequestURI: info.FullMethod,
		Method:     http.MethodPost,
		ClientIP:   address,
		UserAgent:  first(md, userAgentKey),
		Host:       first(md, authorityKey),
		TLS:        tls,
	}}
	meta := requestctx.New(c.fields)
	ctx = requestctx.WithMetadata(context.WithValue(ctx, callRecordKey{}, c), meta)
	// SetHeader fails only on a context carrying no call, which the
	// server's own contexts never are.
	_ = grpc.SetHeader(ctx, metadata.Pairs(traceIDKey, traceID))

	rsp, err := handler(ctx, req)

	if logger.GRPC == nil {
		// A process that never initialized its loggers, which bootstrap
		// always does before Run; its other loggers drop entries too.
		return rsp, err
	}
	st := statusOf(err)
	fields := make([]zapcore.Field, 0, accessLogFieldCap)
	fields = append(
		fields,
		zap.String("status", st.Code().String()),
		zap.String(consts.CTX_METHOD, meta.Method()),
		zap.String(consts.CTX_USERNAME, c.caller.Username),
		zap.String(consts.CTX_USER_ID, c.caller.UserID),
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
