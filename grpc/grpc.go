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

// RegisterServiceFunc 是服务注册函数的类型定义
type RegisterServiceFunc func(*grpc.Server)

// 存储所有等待注册的服务
var serviceRegistrars []RegisterServiceFunc

// RegisterService 添加一个服务注册函数，用于在服务器启动时注册服务
func RegisterService(registrar RegisterServiceFunc) {
	mu.Lock()
	defer mu.Unlock()

	serviceRegistrars = append(serviceRegistrars, registrar)

	// 如果服务器已经初始化，直接注册服务
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

	// 配置消息大小限制
	if cfg.MaxRecvMsgSize > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSize))
	}
	if cfg.MaxSendMsgSize > 0 {
		opts = append(opts, grpc.MaxSendMsgSize(cfg.MaxSendMsgSize))
	}

	// 配置窗口大小
	if cfg.InitialConnWindowSize > 0 {
		opts = append(opts, grpc.InitialConnWindowSize(cfg.InitialConnWindowSize))
	}
	if cfg.InitialWindowSize > 0 {
		opts = append(opts, grpc.InitialWindowSize(cfg.InitialWindowSize))
	}
	// 配置保活参数
	if cfg.KeepaliveTime > 0 || cfg.KeepaliveTimeout > 0 {
		serverParams := keepalive.ServerParameters{
			Time:                  cfg.KeepaliveTime,         // 如果空闲超过此时间，则发送ping
			Timeout:               cfg.KeepaliveTimeout,      // 如果ping在此时间内没有响应，则关闭连接
			MaxConnectionIdle:     cfg.MaxConnectionIdle,     // 如果连接空闲超过此时间，则关闭
			MaxConnectionAge:      cfg.MaxConnectionAge,      // 连接最大年龄
			MaxConnectionAgeGrace: cfg.MaxConnectionAgeGrace, // 强制关闭连接前的宽限期
		}

		// 配置如何处理客户端的保活ping
		enforcementPolicy := keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second, // 如果客户端在此时间内ping超过一次，则断开连接
			PermitWithoutStream: true,            // 允许客户端在没有活跃RPC时发送ping
		}

		opts = append(
			opts,
			grpc.KeepaliveParams(serverParams),
			grpc.KeepaliveEnforcementPolicy(enforcementPolicy),
		)
	}

	// 限制每个连接的最大并发流数
	opts = append(opts, grpc.MaxConcurrentStreams(100))

	// 添加一元拦截器用于日志记录、恢复、认证等
	opts = append(opts, grpc.ChainUnaryInterceptor(
		LoggingUnaryInterceptor,
		RecoveryUnaryInterceptor,
	))

	// 添加流拦截器
	opts = append(opts, grpc.ChainStreamInterceptor(
		LoggingStreamInterceptor,
		RecoveryStreamInterceptor,
	))

	// TLS 配置
	if cfg.TLSEnabled {
		tlsConfig, err := util.BuildTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile, false)
		if err != nil {
			return errors.Wrap(err, "failed to build TLS config")
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.Creds(creds))
	} else {
		// 如果不使用TLS，仍然提供一个insecure凭证避免某些客户端问题
		opts = append(opts, grpc.Creds(insecure.NewCredentials()))
	}

	// 创建服务器
	server = grpc.NewServer(opts...)

	// 注册健康检查服务
	if cfg.HealthCheckEnabled {
		healthServer := health.NewServer()
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		healthpb.RegisterHealthServer(server, healthServer)
	}

	// 注册反射服务（用于 grpcurl 等工具）
	if cfg.ReflectionEnabled {
		reflection.Register(server)
	}

	// 注册之前等待的所有服务
	for _, registrar := range serviceRegistrars {
		registrar(server)
	}

	// 创建监听器但不启动服务
	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Listen, cfg.Port))
	if err != nil {
		return errors.Wrap(err, "failed to create listener")
	}
	listener = l

	initialized = true
	zap.S().Infow("grpc server initialized", "addr", listener.Addr().String())

	return nil
}

// Run 启动 gRPC 服务器并阻塞直到收到终止信号
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

	// 启动 gRPC 服务器
	go func() {
		zap.S().Infow("gRPC server started", "addr", listener.Addr().String())
		if err := server.Serve(listener); err != nil {
			errCh <- errors.Wrap(err, "failed to serve")
		}
	}()

	// 监听启动错误
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

	// 优雅停机
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

	// 重置状态
	server = nil
	listener = nil
	initialized = false
}

// LoggingUnaryInterceptor 用于记录一元RPC调用的日志
func LoggingUnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()

	// 调用真正的处理程序
	resp, err := handler(ctx, req)

	// 记录请求信息
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

// LoggingStreamInterceptor 用于记录流式RPC调用的日志
func LoggingStreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	start := time.Now()

	// 包装流，以便可以获取更多信息
	wrappedStream := &wrappedServerStream{ServerStream: ss}

	// 调用真正的处理程序
	err := handler(srv, wrappedStream)

	// 记录请求信息
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

// RecoveryUnaryInterceptor 处理一元RPC调用中的panic
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

// RecoveryStreamInterceptor 处理流式RPC调用中的panic
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

// wrappedServerStream 包装了 grpc.ServerStream 以便添加更多功能
type wrappedServerStream struct {
	grpc.ServerStream
}

// RegisterStatsHandler 注册统计处理器
func RegisterStatsHandler(handler stats.Handler) {
	mu.Lock()
	defer mu.Unlock()

	if !initialized || server == nil {
		// 服务器尚未初始化，将在Init中添加
		return
	}

	// 这个功能无法在服务器初始化后添加
	zap.S().Warnw("cannot register stats handler after server initialization")
}
