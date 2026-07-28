package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/stats"
)

var (
	initialized bool
	server      *grpc.Server
	mu          sync.RWMutex
	listener    net.Listener
)

// RegisterServiceFunc is the type of a service registration function.
type RegisterServiceFunc func(*grpc.Server)

// serviceRegistrars holds every service that is waiting to be registered.
var serviceRegistrars []RegisterServiceFunc

// RegisterService adds a service registration function, which registers the service when the server starts.
func RegisterService(registrar RegisterServiceFunc) {
	mu.Lock()
	defer mu.Unlock()

	serviceRegistrars = append(serviceRegistrars, registrar)

	// register right away if the server is already initialized
	if initialized && server != nil {
		registrar(server)
	}
}

func Init() error {
	cfg := config.App.Grpc
	if !cfg.Enabled {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	var opts []grpc.ServerOption

	// message size limits
	if cfg.MaxRecvMsgSize > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSize))
	}
	if cfg.MaxSendMsgSize > 0 {
		opts = append(opts, grpc.MaxSendMsgSize(cfg.MaxSendMsgSize))
	}

	// window sizes
	if cfg.InitialConnWindowSize > 0 {
		opts = append(opts, grpc.InitialConnWindowSize(cfg.InitialConnWindowSize))
	}
	if cfg.InitialWindowSize > 0 {
		opts = append(opts, grpc.InitialWindowSize(cfg.InitialWindowSize))
	}
	// keepalive parameters
	if cfg.KeepaliveTime > 0 || cfg.KeepaliveTimeout > 0 {
		serverParams := keepalive.ServerParameters{
			Time:                  cfg.KeepaliveTime,         // send a ping once the connection has been idle this long
			Timeout:               cfg.KeepaliveTimeout,      // close the connection if a ping is not answered within this time
			MaxConnectionIdle:     cfg.MaxConnectionIdle,     // close the connection once it has been idle this long
			MaxConnectionAge:      cfg.MaxConnectionAge,      // maximum age of a connection
			MaxConnectionAgeGrace: cfg.MaxConnectionAgeGrace, // grace period before a connection is closed forcibly
		}

		// how client keepalive pings are handled
		enforcementPolicy := keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second, // disconnect a client that pings more than once within this time
			PermitWithoutStream: true,            // let clients ping while no RPC is active
		}

		opts = append(
			opts,
			grpc.KeepaliveParams(serverParams),
			grpc.KeepaliveEnforcementPolicy(enforcementPolicy),
		)
	}

	// limit the number of concurrent streams per connection
	opts = append(opts, grpc.MaxConcurrentStreams(100))

	// unary interceptors for logging, panic recovery, authentication and so on
	opts = append(opts, grpc.ChainUnaryInterceptor(
		LoggingUnaryInterceptor,
		RecoveryUnaryInterceptor,
	))

	// stream interceptors
	opts = append(opts, grpc.ChainStreamInterceptor(
		LoggingStreamInterceptor,
		RecoveryStreamInterceptor,
	))

	// TLS configuration
	if cfg.TLSEnabled {
		tlsConfig, err := util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, false)
		if err != nil {
			return errors.Wrap(err, "failed to build TLS config")
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.Creds(creds))
	} else {
		// without TLS still provide insecure credentials, some clients misbehave otherwise
		opts = append(opts, grpc.Creds(insecure.NewCredentials()))
	}

	// create the server
	server = grpc.NewServer(opts...)

	// register the health check service
	if cfg.HealthCheckEnabled {
		healthServer := health.NewServer()
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		healthpb.RegisterHealthServer(server, healthServer)
	}

	// register the reflection service (used by tools such as grpcurl)
	if cfg.ReflectionEnabled {
		reflection.Register(server)
	}

	// register every service that has been waiting
	for _, registrar := range serviceRegistrars {
		registrar(server)
	}

	// create the listener without serving on it yet
	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Listen, cfg.Port))
	if err != nil {
		return errors.Wrap(err, "failed to create listener")
	}
	listener = l

	initialized = true
	zap.S().Infow("grpc server initialized", "addr", listener.Addr().String())

	return nil
}

// Run serves the gRPC server in a background goroutine and returns any error reported at startup.
func Run() error {
	mu.Lock()
	defer mu.Unlock()

	if !initialized || server == nil {
		if err := Init(); err != nil {
			return err
		}
	}

	cfg := config.App.Grpc
	if !cfg.Enabled {
		return nil
	}

	errCh := make(chan error, 1)

	// start the gRPC server
	go func() {
		zap.S().Infow("gRPC server started", "addr", listener.Addr().String())
		if err := server.Serve(listener); err != nil {
			errCh <- errors.Wrap(err, "failed to serve")
		}
	}()

	// check for a startup error
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if server == nil {
		return
	}

	zap.S().Infow("gRPC server shutdown initiated")

	// graceful shutdown
	gracefulStopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		zap.S().Infow("gRPC server shutdown completed gracefully")
	case <-gracefulStopCtx.Done():
		zap.S().Warnw("gRPC server shutdown timeout, forcing shutdown")
		server.Stop()
	}

	// reset the state
	server = nil
	listener = nil
	initialized = false
}

// LoggingUnaryInterceptor logs unary RPC calls.
func LoggingUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()

	// call the real handler
	resp, err := handler(ctx, req)

	// log the request
	logger := zap.S().With(
		"method", info.FullMethod,
		"duration", time.Since(start),
	)

	if err != nil {
		logger.Errorw("grpc unary call failed", "error", err)
	} else {
		logger.Infow("grpc unary call completed")
	}

	return resp, err
}

// LoggingStreamInterceptor logs streaming RPC calls.
func LoggingStreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	start := time.Now()

	// wrap the stream so that more information is available
	wrappedStream := &wrappedServerStream{ServerStream: ss}

	// call the real handler
	err := handler(srv, wrappedStream)

	// log the request
	logger := zap.S().With(
		"method", info.FullMethod,
		"duration", time.Since(start),
		"isClientStream", info.IsClientStream,
		"isServerStream", info.IsServerStream,
	)

	if err != nil {
		logger.Errorw("grpc stream call failed", "error", err)
	} else {
		logger.Infow("grpc stream call completed")
	}

	return err
}

// RecoveryUnaryInterceptor recovers from panics raised during unary RPC calls.
func RecoveryUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if r := recover(); r != nil {
			zap.S().Errorw(
				"grpc unary call panic recovered",
				"method", info.FullMethod,
				"panic", r,
			)
			err = errors.Errorf("internal server error: %v", r)
		}
	}()

	return handler(ctx, req)
}

// RecoveryStreamInterceptor recovers from panics raised during streaming RPC calls.
func RecoveryStreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			zap.S().Errorw(
				"grpc stream call panic recovered",
				"method", info.FullMethod,
				"panic", r,
			)
			err = errors.Errorf("internal server error: %v", r)
		}
	}()

	return handler(srv, ss)
}

// wrappedServerStream wraps grpc.ServerStream so that more behavior can be added to it.
type wrappedServerStream struct {
	grpc.ServerStream
}

// RegisterStatsHandler registers a stats handler.
func RegisterStatsHandler(handler stats.Handler) {
	mu.Lock()
	defer mu.Unlock()

	if !initialized || server == nil {
		// the server is not initialized yet, there is nothing to register the handler on
		return
	}

	// a stats handler cannot be added once the server has been initialized
	zap.S().Warnw("cannot register stats handler after server initialization")
}
