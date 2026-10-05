package grpcserver

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/hydroan/gst/internal/consts"
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

// requestScope gives a unary call what the HTTP listener's tracing and
// access-log middleware give a request (a stream gets the same from
// requestScopeStream, both through enterCall and callScope.leave). It
// stamps the call's trace id on the context as the identity of the
// execution — the server span's when tracing is on, the span being the
// root the call's inner spans hang off; the caller's x-trace-id otherwise,
// which with tracing on seeds the span's trace id the way the X-Trace-ID
// header does over HTTP; or a generated one — and publishes it in the
// response header, which goes out with the status of a failed call as
// well; attaches the request metadata a ServiceContext built on the
// context answers for, and keeps the call record an authentication
// interceptor later adds the caller to (see WithCaller); and, once the
// handler returns, writes the call's entry to the access log with the
// fields the HTTP entry carries, the caller as established by then, the
// status being the code's name and, for a failed call, the status message
// beside it.
//
// The metadata names the action the call runs the way the HTTP listener
// names a request's: the route and HTTP method the registration described
// the rpc with, /api/records/:id and GET, STREAM for a Stream action (see
// Method), so a hook, a log or a span reads the same route and method
// whichever listener served the action; the full method as path and
// request URI, the call's target on the wire the way a request's path is;
// the address of the peer and whether it speaks TLS, the authority the call
// was addressed to and the user agent; and whether the method requires
// auth, as the registration described it (see Method.Public). A method the
// registration described with no action, the health and reflection
// services' among them, keeps the full method as its route and POST, the
// method every gRPC call is on the wire, as its method. The forwarding
// headers of a proxy in front are not read: the HTTP listener believes the
// client address they carry from the peers server.trusted_proxies names
// alone, a judgement gin makes for it and this listener has no gin to make,
// and the protocol they carry from any peer.
func requestScope(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	scope := enterCall(ctx, info.FullMethod)
	rsp, err := handler(scope.ctx, req)
	scope.leave(err)
	return rsp, err
}

// requestScopeStream is requestScope for a stream: the scope is entered
// once, ahead of the first message, the handler gets the stream on the
// scoped context, and the access-log entry is written once the stream is
// over, with the status it ended with.
func requestScopeStream(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	scope := enterCall(ss.Context(), info.FullMethod)
	err := handler(srv, withStreamContext(scope.ctx, ss))
	scope.leave(err)
	return err
}

// callScope is a call inside its request scope: the context the handler
// runs on and what the access-log entry written when the call ends needs.
type callScope struct {
	ctx        context.Context
	fullMethod string
	record     *callRecord
	meta       requestctx.Metadata
	traceID    string
	start      time.Time
}

// serviceOf returns the service of the full method name of an rpc, the
// part between its slashes: grpc.health.v1.Health of
// /grpc.health.v1.Health/Check.
func serviceOf(fullMethod string) string {
	service, _, _ := strings.Cut(strings.TrimPrefix(fullMethod, "/"), "/")
	return service
}

// enterCall enters the request scope of the call of fullMethod on ctx (see
// requestScope): the trace id, the request metadata and the call record
// go on the context, and the trace id goes out in the response header.
func enterCall(ctx context.Context, fullMethod string) *callScope {
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
	method := methods[fullMethod]
	route, httpMethod := fullMethod, http.MethodPost
	if method.Route != "" {
		route = method.Route
	}
	if method.HTTPMethod != "" {
		httpMethod = method.HTTPMethod
	}
	c := &callRecord{method: method, fields: requestctx.Fields{
		Route:        route,
		Path:         fullMethod,
		RequestURI:   fullMethod,
		Method:       httpMethod,
		ClientIP:     address,
		UserAgent:    first(md, userAgentKey),
		Host:         first(md, authorityKey),
		TLS:          tls,
		RequiresAuth: requiresAuth(serviceOf(fullMethod), fullMethod),
	}}
	meta := requestctx.New(c.fields)
	ctx = requestctx.WithMetadata(context.WithValue(ctx, callRecordKey{}, c), meta)
	// SetHeader fails only on a context carrying no call, which the
	// server's own contexts never are.
	_ = grpc.SetHeader(ctx, metadata.Pairs(traceIDKey, traceID))
	return &callScope{ctx: ctx, fullMethod: fullMethod, record: c, meta: meta, traceID: traceID, start: start}
}

// leave writes the access-log entry of the call, which ended with err.
func (s *callScope) leave(err error) {
	if logger.GRPC == nil {
		// A process that never initialized its loggers, which bootstrap
		// always does before Run; its other loggers drop entries too.
		return
	}
	st := statusOf(err)
	fields := make([]zapcore.Field, 0, accessLogFieldCap)
	fields = append(
		fields,
		zap.String("status", st.Code().String()),
		zap.String(consts.CTX_METHOD, s.meta.Method()),
		zap.String(consts.CTX_USERNAME, s.record.caller.Username),
		zap.String(consts.CTX_USER_ID, s.record.caller.UserID),
		zap.String(consts.TRACE_ID, s.traceID),
		zap.String(consts.CTX_ROUTE, s.meta.Route()),
		zap.String(consts.CTX_PATH, s.meta.Path()),
		zap.String("ip", s.meta.ClientIP()),
		zap.String("user_agent", s.meta.UserAgent()),
		util.LogDuration(time.Since(s.start)),
	)
	if err != nil {
		fields = append(fields, zap.String("error", st.Message()))
	}
	logger.GRPC.Info(s.fullMethod, fields...)
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
