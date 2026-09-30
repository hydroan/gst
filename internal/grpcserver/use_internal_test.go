package grpcserver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// TestUseRunsTheProjectInterceptorsInsideTheBuiltinChain pins where the
// interceptors a project registers run: inside the framework's own chain,
// the common ones before the auth ones and each group in registration
// order, and around the handler.
func TestUseRunsTheProjectInterceptorsInsideTheBuiltinChain(t *testing.T) {
	reset(t)
	var mu sync.Mutex
	var order []string
	note := func(name string) Interceptor {
		return func(ctx context.Context) (context.Context, error) {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return ctx, nil
		}
	}
	UseAuth(note("auth"))
	Use(note("common-1"), note("common-2"))
	serve(map[string]func(context.Context) error{"Ping": func(context.Context) error {
		mu.Lock()
		order = append(order, "handler")
		mu.Unlock()
		return nil
	}})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Ping"))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"common-1", "common-2", "auth", "handler"}, order)
}

// TestUseAuthSkipsThePublicMethods pins that the auth interceptors leave the
// methods a service registers as public alone and guard every other one,
// the way the HTTP listener keeps Public() routes out of the authenticated
// group.
func TestUseAuthSkipsThePublicMethods(t *testing.T) {
	reset(t)
	UseAuth(func(context.Context) (context.Context, error) {
		return nil, status.Error(codes.Unauthenticated, "no credentials")
	})
	serve(map[string]func(context.Context) error{
		"Ping": func(context.Context) error { return nil },
		"Look": func(context.Context) error { return nil },
	}, Method{Name: "/gst.test.Echo/Look", Public: true}, Method{Name: "/gst.test.Echo/Ping"})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Look"), "a public method needs no credentials")
	require.Equal(t, codes.Unauthenticated, status.Code(call(ctx, conn, "Ping")))
}

// TestUseAuthLeavesTheServersOwnServicesAlone pins that the interceptors
// UseAuth queued guard the project's methods alone: with one refusing every
// call, the health service still answers a check and a watch, the
// reflection service still lists the services, and the project's method is
// refused.
func TestUseAuthLeavesTheServersOwnServicesAlone(t *testing.T) {
	reset(t)
	UseAuth(func(context.Context) (context.Context, error) {
		return nil, status.Error(codes.Unauthenticated, "no credentials")
	})
	serve(map[string]func(context.Context) error{"Ping": func(context.Context) error { return nil }}, Method{Name: "/gst.test.Echo/Ping"})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, healthOf(t, conn, ""))
	watch, err := grpc_health_v1.NewHealthClient(conn).Watch(ctx, &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	first, err := watch.Recv()
	require.NoError(t, err, "the health watch, a stream, takes no credentials either")
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, first.GetStatus())
	names, err := services(t, conn)
	require.NoError(t, err)
	require.Contains(t, names, "gst.test.Echo")
	require.Equal(t, codes.Unauthenticated, status.Code(call(ctx, conn, "Ping")))
}

// TestUseLeavesTheServersOwnServicesAlone pins that the common interceptors
// leave the health and reflection services alone the way the auth
// interceptors do (see TestUseAuthLeavesTheServersOwnServicesAlone): they
// are the framework's, not the project's actions, the way the HTTP
// listener's probes run outside the middleware a project registers; a
// common interceptor refusing every call it sees still leaves a probe its
// answer.
func TestUseLeavesTheServersOwnServicesAlone(t *testing.T) {
	reset(t)
	Use(func(context.Context) (context.Context, error) {
		return nil, status.Error(codes.PermissionDenied, "refused")
	})
	serve(map[string]func(context.Context) error{"Ping": func(context.Context) error { return nil }}, Method{Name: "/gst.test.Echo/Ping"})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, healthOf(t, conn, ""))
	watch, err := grpc_health_v1.NewHealthClient(conn).Watch(ctx, &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	first, err := watch.Recv()
	require.NoError(t, err, "the health watch, a stream, runs outside the common interceptors too")
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, first.GetStatus())
	names, err := services(t, conn)
	require.NoError(t, err)
	require.Contains(t, names, "gst.test.Echo")
	require.Equal(t, codes.PermissionDenied, status.Code(call(ctx, conn, "Ping")), "the project's own methods stay intercepted")
}

// TestAPanicInAProjectInterceptorIsRecovered pins that the framework's
// recovery wraps the project's interceptors as it wraps the handler: the
// caller gets codes.Internal, the panic is logged and the server goes on.
func TestAPanicInAProjectInterceptorIsRecovered(t *testing.T) {
	reset(t)
	Use(func(context.Context) (context.Context, error) { panic("boom") })
	echo(nil, nil)
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := call(ctx, conn, "Ping")

	require.Equal(t, codes.Internal, status.Code(err))
	require.Len(t, recoveryLog.All(), 1)
	require.Equal(t, codes.Internal, status.Code(call(ctx, conn, "Ping")), "the server must outlive the panic")
}

// TestUseAfterRunPanics pins that an interceptor registered once the server
// runs is a programming error, reported at once rather than left out of the
// chain quietly, like a service registered late.
func TestUseAfterRunPanics(t *testing.T) {
	reset(t)
	echo(nil, nil)
	start(t)
	pass := func(ctx context.Context) (context.Context, error) { return ctx, nil }

	require.Panics(t, func() { Use(pass) })
	require.Panics(t, func() { UseAuth(pass) })
}
