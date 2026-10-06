package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/middleware"
	"github.com/stretchr/testify/require"
)

// TestRegisterAfterRunPanics pins that a middleware registered once the
// server runs is a programming error reported at once rather than left off
// the routes quietly, the way grpcserver.Use reports an interceptor
// registered late: gin fixes the chain of a route as the route registers,
// so a middleware arriving later would guard nothing. The test server of
// this package runs already; the flag is set and restored all the same, so
// the test holds whatever brought it up.
func TestRegisterAfterRunPanics(t *testing.T) {
	was := started.Load()
	started.Store(1)
	t.Cleanup(func() { started.Store(was) })
	pass := func(*gin.Context) {}

	require.PanicsWithValue(t, "router: middleware.Register after the server started; register middleware at package initialization", func() { middleware.Register(pass) })
	require.PanicsWithValue(t, "router: middleware.RegisterAuth after the server started; register middleware at package initialization", func() { middleware.RegisterAuth(pass) })
}
