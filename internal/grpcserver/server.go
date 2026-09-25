// Package grpcserver serves the gRPC services of the models declaring
// GRPC(), on a listener of its own beside the HTTP one and on the same
// lifecycle: bootstrap starts it with the other listeners, drains it with
// the readiness probe and stops it within the shutdown's window. The
// services are the ones the generated pb/pb.gen.go registers (see Register);
// with none registered the listener never opens, so a project without
// gRPC has no port to expose and nothing to switch off.
package grpcserver

import (
	"context"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/cockroachdb/errors"
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
	// healthServer answers the standard health checks on server; Drain
	// turns it to NOT_SERVING.
	healthServer *health.Server

	// listened, when set, is told the address the listener bound; a test
	// seam, nothing in the framework sets it.
	listened func(net.Addr)

	// drainTimeout bounds how long Stop waits for the calls in flight, the
	// shutdown's one window shared with the HTTP listener, the components
	// and the providers. A variable so a test can play the bound out in
	// milliseconds.
	drainTimeout = lifecycle.StopTimeout
)

// Register queues fn to register a service on the server Run starts, the
// way the generated pb/pb.gen.go registers the service of every model
// declaring GRPC(): fn gets the server as a grpc.ServiceRegistrar and calls
// the RegisterXxxServiceServer function the protobuf plugin generated. It
// runs at package initialization, before bootstrap starts the listeners;
// registering once the server runs would serve nothing, so it panics.
func Register(fn func(grpc.ServiceRegistrar)) {
	mu.Lock()
	defer mu.Unlock()
	if started.Load() {
		panic("grpcserver: Register after the server started; register services at package initialization")
	}
	registrations = append(registrations, fn)
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
	cfg := config.App.GRPC
	opts := []grpc.ServerOption{grpc.KeepaliveParams(keepalive.ServerParameters{Time: cfg.KeepaliveTime, Timeout: cfg.KeepaliveTimeout})}
	if cfg.TLSEnabled {
		creds, err := credentials.NewServerTLSFromFile(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return errors.Wrap(err, "load the grpc server certificate")
		}
		opts = append(opts, grpc.Creds(creds))
	}
	srv := grpc.NewServer(opts...)
	for _, register := range registrations {
		register(srv)
	}
	// The health service answers SERVING from the start and NOT_SERVING
	// from Drain on, the readiness the HTTP probe reports; the reflection
	// service lets grpcurl and its kind list what the server carries.
	healthServer = health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, healthServer)
	if cfg.Reflection {
		reflection.Register(srv)
	}

	addr := net.JoinHostPort(cfg.Listen, strconv.Itoa(cfg.Port))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return errors.Wrapf(err, "listen on %s for grpc", addr)
	}
	server = srv
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

// Drain turns the health service to NOT_SERVING, so a balancer checking it
// stops sending traffic here, the way controller.Probe.Drain fails the HTTP
// readiness probe; the services keep answering what still arrives. Bootstrap
// calls both when the process is told to stop, ahead of the shutdown delay.
func Drain() {
	mu.Lock()
	defer mu.Unlock()
	if healthServer != nil {
		healthServer.Shutdown()
	}
}

// Stop shuts the server down: it stops accepting connections and waits for
// the calls in flight, for up to drainTimeout and no longer than abandon
// lasts — not at all when it has already ended, for a process that must not
// wait on anything, see lifecycle.FailNow. The connections a drain cut
// short are closed, their calls ended with an error, rather than left to
// run past the teardown of what they use; a handler that ignores its
// context's cancellation is then left to end on its own, the way an HTTP
// handler is after Close. Neither wait holds Stop past the bound: grpc's
// GracefulStop waits for every handler while holding the server's lock, so
// a handler that never returns would hold a forced Stop behind that lock
// too, and the forced stop therefore runs on a goroutine of its own.
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
	select {
	case <-drained:
		log.Infow("grpc server shutdown completed")
	case <-ctx.Done():
		log.Warnw("grpc server closing the connections its drain left open", "reason", context.Cause(ctx))
		go srv.Stop()
	}
	server = nil
	healthServer = nil
}
