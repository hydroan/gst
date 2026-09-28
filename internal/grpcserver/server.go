// Package grpcserver serves the gRPC services of the models declaring
// GRPC(), on a listener of its own beside the HTTP one and on the same
// lifecycle: bootstrap starts it with the other listeners, drains it with
// the readiness probe and stops it side by side with the HTTP listener
// within the shutdown's window, its streams ending as the stop begins. The
// services are the ones the generated pb.gen.go files register (see Register);
// with none registered the listener never opens, so a project without
// gRPC has no port to expose and nothing to switch off.
package grpcserver

import (
	"context"
	"math"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/cockroachdb/errors"
	"github.com/dustin/go-humanize"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/lifecycle"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

var (
	// mu guards registrations and the running server.
	mu sync.Mutex
	// registrations are the functions Register queued, run on the server
	// Run builds, in order.
	registrations []func(grpc.ServiceRegistrar)
	// started records that Run built the server, from when Register is a
	// programming error.
	started atomic.Bool
	// server is the running server, nil before Run and after Stop.
	server *grpc.Server
	// healthServer answers the standard health checks on server, for the
	// process and for each service the server carries; Drain turns every
	// status to NOT_SERVING.
	healthServer *health.Server
	// beginStop ends the context every stream of server watches, the moment
	// Stop begins (see streamShutdown); nil before Run and after Stop.
	beginStop context.CancelFunc

	// listened, when set, is told the address the listener bound; a test
	// seam, nothing in the framework sets it.
	listened func(net.Addr)

	// drainTimeout bounds how long Stop waits for the calls in flight: the
	// window the shutdown stops this listener and the HTTP one within, side
	// by side (see bootstrap), the components getting a window of their own
	// after it. A variable so a test can play the bound out in milliseconds.
	drainTimeout = lifecycle.StopTimeout
)

// Register queues fn to register a service on the server Run starts, the
// way the generated pb.gen.go of every package under pb/ registers the
// services of the models declaring GRPC(): fn gets the server as a
// grpc.ServiceRegistrar and calls
// the RegisterXxxServiceServer function the protobuf plugin generated, and
// methods describe the service's rpcs (see Method): which are public and
// what the same actions are over HTTP. It runs at package initialization,
// before bootstrap starts the listeners; registering once the server runs
// would serve nothing, so it panics.
func Register(fn func(grpc.ServiceRegistrar), described ...Method) {
	mu.Lock()
	defer mu.Unlock()
	if started.Load() {
		panic("grpcserver: Register after the server started; register services at package initialization")
	}
	registrations = append(registrations, fn)
	if methods == nil {
		methods = make(map[string]Method, len(described))
	}
	for _, m := range described {
		methods[m.Name] = m
	}
}

// HasServices reports whether a service was registered: Run opens the
// listener only then, which is what a test harness waiting for the listener
// needs to know.
func HasServices() bool {
	mu.Lock()
	defer mu.Unlock()
	return len(registrations) > 0
}

// Run serves the registered services on the address config.App.GRPC names
// until Stop. With no service registered it returns at once and opens no
// listener. Like router.Run it is one of bootstrap's long-running
// functions: an error it returns, a port it cannot bind or a certificate it
// cannot load, brings the process down.
func Run() error {
	mu.Lock()
	defer mu.Unlock()
	log := zap.S()
	if len(registrations) == 0 {
		log.Debugw("grpc server not started: no service registered")
		return nil
	}
	if err := registerServerMetrics(); err != nil {
		return err
	}
	cfg := config.App.GRPC
	// The streams of the server watch stopping, which Stop ends; a Run
	// that fails before serving ends it itself.
	stopping, stop := context.WithCancel(context.Background())
	failed := func(err error) error {
		stop()
		return err
	}
	// The keys of the grpc section are grpc-go's own parameters, handed
	// over as they are: zero where unset, which grpc-go fills with its own
	// defaults.
	opts := append(chains(stopping),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:                  cfg.KeepaliveTime,
			Timeout:               cfg.KeepaliveTimeout,
			MaxConnectionAge:      cfg.MaxConnectionAge,
			MaxConnectionAgeGrace: cfg.MaxConnectionAgeGrace,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             cfg.KeepaliveMinTime,
			PermitWithoutStream: cfg.KeepalivePermitWithoutStream,
		}),
	)
	if cfg.MaxRecvMsgSize != "" {
		size, err := humanize.ParseBytes(cfg.MaxRecvMsgSize)
		if err != nil {
			return failed(errors.Wrapf(err, "parse grpc.max_recv_msg_size %q", cfg.MaxRecvMsgSize))
		}
		// A message's length is a 32-bit field of the wire, and no message
		// at all would fit under zero.
		if size == 0 || size > math.MaxInt32 {
			return failed(errors.Newf("grpc.max_recv_msg_size %q is not between 1 byte and %s", cfg.MaxRecvMsgSize, humanize.IBytes(math.MaxInt32)))
		}
		opts = append(opts, grpc.MaxRecvMsgSize(int(size)))
	}
	if cfg.TLSEnabled {
		creds, err := credentials.NewServerTLSFromFile(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return failed(errors.Wrap(err, "load the grpc server certificate"))
		}
		opts = append(opts, grpc.Creds(creds))
	}
	srv := grpc.NewServer(opts...)
	for _, register := range registrations {
		register(srv)
	}
	// A non-public method with no auth interceptor is served to anyone. The
	// HTTP listener warns the same way for a route of the authenticated
	// group with nothing registered on it (see router.Run), and a project
	// may authenticate in an interceptor registered for every method, so
	// this is a warning and not a refusal.
	if unguarded := unguardedMethods(); len(unguarded) > 0 && len(authInterceptors) == 0 {
		log.Warnw("grpc server serves non-public methods with no auth interceptor registered", "methods", unguarded)
	}
	// The health service answers SERVING from the start and NOT_SERVING
	// from Drain on, the readiness the HTTP probe reports, for the process
	// and for each service the server carries by name, so that a probe
	// naming a service, the way a Kubernetes gRPC probe may, is answered;
	// the reflection service lets grpcurl and its kind list what the server
	// carries.
	healthServer = health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, healthServer)
	if cfg.Reflection {
		reflection.Register(srv)
	}
	for name := range srv.GetServiceInfo() {
		healthServer.SetServingStatus(name, grpc_health_v1.HealthCheckResponse_SERVING)
	}
	// Every method's series exist from the start, at zero, so a dashboard
	// finds them before the first call.
	serverMetrics.InitializeMetrics(srv)

	addr := net.JoinHostPort(cfg.Listen, strconv.Itoa(cfg.Port))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return failed(errors.Wrapf(err, "listen on %s for grpc", addr))
	}
	server = srv
	beginStop = stop
	started.Store(true)
	if listened != nil {
		listened(lis.Addr())
	}
	log.Infow("grpc server started", "addr", lis.Addr().String(), "tls", cfg.TLSEnabled, "reflection", cfg.Reflection)
	// Serve holds the lock's callers off until it returns, so it runs
	// unlocked: Stop takes the lock to end it.
	mu.Unlock()
	err = srv.Serve(lis)
	mu.Lock()
	if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Errorw("grpc server failed", "err", err)
		return err
	}
	return nil
}

// Drain turns the health service to NOT_SERVING, for the process and every
// service it names, so a balancer checking it stops sending traffic here,
// the way controller.Probe.Drain fails the HTTP readiness probe; the
// services keep answering what still arrives. Bootstrap calls both when the
// process is told to stop, ahead of the shutdown delay.
func Drain() {
	mu.Lock()
	defer mu.Unlock()
	if healthServer != nil {
		healthServer.Shutdown()
	}
}

// Stop shuts the server down: it stops accepting connections, ends every
// stream's context so the stream is answered Unavailable and its client
// resumes elsewhere (see streamShutdown), and waits for the unary calls in
// flight, for up to drainTimeout and no longer than abandon lasts — not at
// all when it has already ended, for a process that must not wait on
// anything, see lifecycle.FailNow. The connections a drain cut short are
// closed, their calls ended with an error, rather than left to run past the
// teardown of what they use; a handler that ignores its context's
// cancellation is then left to end on its own, the way an HTTP handler is
// after Close. Neither wait holds Stop past the bound: grpc's GracefulStop
// waits for every handler while holding the server's lock, so a handler
// that never returns would hold a forced Stop behind that lock too, and the
// forced stop therefore runs on a goroutine of its own.
func Stop(abandon context.Context) {
	mu.Lock()
	defer mu.Unlock()
	if server == nil {
		return
	}
	log := zap.S()
	log.Infow("grpc server shutdown initiated")
	ctx, cancel := context.WithTimeout(abandon, drainTimeout)
	defer cancel()
	srv := server
	drained := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(drained)
	}()
	beginStop()
	select {
	case <-drained:
		log.Infow("grpc server shutdown completed")
	case <-ctx.Done():
		log.Warnw("grpc server closing the connections its drain left open", "reason", context.Cause(ctx))
		go srv.Stop()
	}
	server = nil
	healthServer = nil
	beginStop = nil
}
