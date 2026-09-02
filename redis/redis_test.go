package redis_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/redis"
	"github.com/stretchr/testify/require"
)

// TestHealthReportsReachableServer asserts the readiness answer a connected
// process gives.
func TestHealthReportsReachableServer(t *testing.T) {
	if err := redis.Health(t.Context()); err != nil {
		t.Fatalf("want a healthy connection, got %v", err)
	}
}

// TestInitInstrumentsTracingWithoutCallerAttributes pins how Init instruments
// the client: every command still exports a span carrying the raw statement,
// which is what a trace is read for, but none of the code.* caller
// attributes, which redisotel would resolve to the framework's own redis
// helpers rather than to the caller, at the cost of a stack walk per command.
func TestInitInstrumentsTracingWithoutCallerAttributes(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	// The client TestMain connected was instrumented before any tracer
	// provider existed; reconnecting binds the instrumentation to the one
	// this test installed.
	require.NoError(t, redis.Close())
	require.NoError(t, redis.Init())

	key := "redis_test:tracing:" + t.Name()
	require.NoError(t, redis.Set(t.Context(), key, "value", time.Minute))
	t.Cleanup(func() { _ = redis.Del(t.Context(), key) })

	span := oteltest.EndedNamed(t, recorder, "set")
	keys := make([]string, 0, len(span.Attributes()))
	for _, kv := range span.Attributes() {
		keys = append(keys, string(kv.Key))
	}
	require.Contains(t, keys, "db.statement", "the statement is what a redis span is read for")
	for _, key := range keys {
		require.False(t, strings.HasPrefix(key, "code."), "caller attribute %q must not be recorded", key)
	}
}
