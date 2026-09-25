package grpcserver

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// The tests share the package's state, so they reset it and run one at a
// time.

// accessLog and recoveryLog record what the server logs during a test, in
// place of the file loggers Init builds.
var accessLog, recoveryLog *observer.ObservedLogs

// reset clears what a previous test registered or started, and points the
// server's loggers at fresh recorders.
func reset(t *testing.T) {
	t.Helper()
	Stop(context.Background())
	registrations = nil
	publicMethods = nil
	commonInterceptors = nil
	authInterceptors = nil
	started.Store(false)
	drainTimeout = lifecycle.StopTimeout
	config.App.GRPC = config.GRPC{Listen: "127.0.0.1", Reflection: true}
	accessLog = observe(t, &logger.GRPC)
	recoveryLog = observe(t, &logger.Recovery)
}

// observe swaps *target for a logger recording into the returned recorder
// until the test ends.
func observe(t *testing.T, target **zap.Logger) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	saved := *target
	*target = zap.New(core)
	t.Cleanup(func() { *target = saved })
	return logs
}

// serve registers the service gst.test.Echo with one unary rpc per entry
// of handlers, named by its key, and the full method names in public as
// its public methods. Each rpc takes and answers an empty message,
// answering with the error its handler returns, and runs the server's
// interceptors first, the way the code the protobuf plugin generates does.
func serve(handlers map[string]func(ctx context.Context) error, public ...string) {
	methods := make([]grpc.MethodDesc, 0, len(handlers))
	for name, handle := range handlers {
		fullMethod := "/gst.test.Echo/" + name
		methods = append(methods, grpc.MethodDesc{
			MethodName: name,
			Handler: func(_ any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
				in := new(emptypb.Empty)
				if err := dec(in); err != nil {
					return nil, err
				}
				handler := func(ctx context.Context, _ any) (any, error) {
					if err := handle(ctx); err != nil {
						return nil, err
					}
					return &emptypb.Empty{}, nil
				}
				if interceptor == nil {
					return handler(ctx, in)
				}
				return interceptor(ctx, in, &grpc.UnaryServerInfo{FullMethod: fullMethod}, handler)
			},
		})
	}
	Register(func(r grpc.ServiceRegistrar) {
		r.RegisterService(&grpc.ServiceDesc{
			ServiceName: "gst.test.Echo",
			HandlerType: (*any)(nil),
			Methods:     methods,
			Metadata:    "gst/test/echo.proto",
		}, nil)
	}, public...)
}

// echo registers the rpc Ping, answering an empty message with an empty
// message. A call sends on entered when it begins, and, given release,
// holds until release is closed.
func echo(entered chan<- struct{}, release <-chan struct{}) {
	serve(map[string]func(context.Context) error{"Ping": func(context.Context) error {
		if entered != nil {
			entered <- struct{}{}
		}
		if release != nil {
			<-release
		}
		return nil
	}})
}

// start runs Run in a goroutine and returns the address the listener
// bound; Stop runs when the test ends.
func start(t *testing.T) string {
	t.Helper()
	bound := make(chan net.Addr, 1)
	listened = func(addr net.Addr) { bound <- addr }
	t.Cleanup(func() { listened = nil })
	errs := make(chan error, 1)
	go func() { errs <- Run() }()
	select {
	case addr := <-bound:
		t.Cleanup(func() { Stop(context.Background()) })
		return addr.String()
	case err := <-errs:
		t.Fatalf("Run returned before listening: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the listener did not open in time")
	}
	return ""
}

// dial connects to addr with creds, insecure by default.
func dial(t *testing.T, addr string, creds credentials.TransportCredentials) *grpc.ClientConn {
	t.Helper()
	if creds == nil {
		creds = insecure.NewCredentials()
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// call invokes the rpc name of gst.test.Echo over conn.
func call(ctx context.Context, conn *grpc.ClientConn, name string, opts ...grpc.CallOption) error {
	return conn.Invoke(ctx, "/gst.test.Echo/"+name, &emptypb.Empty{}, &emptypb.Empty{}, opts...)
}

// healthOf checks the server's overall health over conn.
func healthOf(t *testing.T, conn *grpc.ClientConn) grpc_health_v1.HealthCheckResponse_ServingStatus {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rsp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	return rsp.GetStatus()
}

// services lists the services the server reflection service names.
func services(t *testing.T, conn *grpc.ClientConn) ([]string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := grpc_reflection_v1.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	if err != nil {
		return nil, err
	}
	if err = stream.Send(&grpc_reflection_v1.ServerReflectionRequest{MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_ListServices{}}); err != nil {
		return nil, err
	}
	rsp, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, s := range rsp.GetListServicesResponse().GetService() {
		names = append(names, s.GetName())
	}
	return names, nil
}

// TestRunOpensNoListenerWithoutAService pins that a process registering no
// gRPC service exposes no port: Run returns at once, having listened on
// nothing, and Stop has nothing to stop.
func TestRunOpensNoListenerWithoutAService(t *testing.T) {
	reset(t)
	listened = func(net.Addr) { t.Error("a listener opened without a service") }
	t.Cleanup(func() { listened = nil })

	require.NoError(t, Run())
	require.Nil(t, server)
	Stop(context.Background())
}

// TestRunServesTheRegisteredServicesWithHealthAndReflection pins what the
// listener carries: the registered service, the standard health service
// answering SERVING until Drain turns it to NOT_SERVING, and the reflection
// service listing them all.
func TestRunServesTheRegisteredServicesWithHealthAndReflection(t *testing.T) {
	reset(t)
	echo(nil, nil)
	conn := dial(t, start(t), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, call(ctx, conn, "Ping"))
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, healthOf(t, conn))
	names, err := services(t, conn)
	require.NoError(t, err)
	require.Contains(t, names, "gst.test.Echo")
	require.Contains(t, names, "grpc.health.v1.Health")

	Drain()

	require.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, healthOf(t, conn))
	require.NoError(t, call(ctx, conn, "Ping"), "draining fails readiness, the service itself keeps answering")
}

// TestRunLeavesReflectionOutWhenDisabled pins the reflection switch.
func TestRunLeavesReflectionOutWhenDisabled(t *testing.T) {
	reset(t)
	config.App.GRPC.Reflection = false
	echo(nil, nil)
	conn := dial(t, start(t), nil)

	_, err := services(t, conn)

	require.Equal(t, codes.Unimplemented, status.Code(err))
}

// TestStopCutsTheCallsItsDrainLeftRunning pins that Stop waits for the
// calls in flight up to drainTimeout and then closes the connections they
// hold, the way the HTTP listener does: a call the drain cannot finish ends
// with an error instead of holding the teardown.
func TestStopCutsTheCallsItsDrainLeftRunning(t *testing.T) {
	reset(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	echo(entered, release)
	conn := dial(t, start(t), nil)
	drainTimeout = 200 * time.Millisecond

	result := make(chan error, 1)
	go func() { result <- call(context.Background(), conn, "Ping") }()
	<-entered

	begin := time.Now()
	Stop(context.Background())

	require.Less(t, time.Since(begin), 5*time.Second, "Stop must not wait past drainTimeout")
	require.Nil(t, server)
	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the cut call did not end")
	}
	// The handler the cut left running ends on its own once released, and
	// its access-log entry is the last thing it writes; the test outlives it.
	close(release)
	require.Eventually(t, func() bool { return accessLog.Len() == 1 }, 5*time.Second, 10*time.Millisecond)
}

// TestRunServesTLSWhenEnabled pins that with tls_enabled the listener speaks
// TLS with the configured certificate: a TLS client is answered and its
// calls report TLS, a plaintext one is not.
func TestRunServesTLSWhenEnabled(t *testing.T) {
	reset(t)
	config.App.GRPC.TLSEnabled = true
	config.App.GRPC.CertFile, config.App.GRPC.KeyFile = selfSigned(t)
	seen := make(chan observed, 1)
	look(seen)
	addr := start(t)

	secure := dial(t, addr, credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})) // a self-signed test certificate
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, healthOf(t, secure))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, call(ctx, secure, "Look"))
	require.True(t, (<-seen).meta.TLS())

	plain := dial(t, addr, nil)
	require.Error(t, call(ctx, plain, "Look"))
}

// TestRegisterAfterRunPanics pins that a service registered once the
// server runs is a programming error, reported at once rather than served
// by nothing.
func TestRegisterAfterRunPanics(t *testing.T) {
	reset(t)
	echo(nil, nil)
	start(t)

	require.Panics(t, func() { echo(nil, nil) })
}

// TestRunFailsOnAnAddressItCannotBind pins that a listener that cannot open
// reports its error, which bootstrap turns into the process failing, instead
// of serving nothing quietly.
func TestRunFailsOnAnAddressItCannotBind(t *testing.T) {
	reset(t)
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = taken.Close() })
	addr, ok := taken.Addr().(*net.TCPAddr)
	require.True(t, ok)
	config.App.GRPC.Port = addr.Port
	echo(nil, nil)

	require.Error(t, Run())
}

// selfSigned writes a self-signed certificate and its key for 127.0.0.1
// into a temporary directory and returns their paths.
func selfSigned(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "gst test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	return certFile, keyFile
}
