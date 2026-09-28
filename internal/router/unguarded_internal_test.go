package router

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestRunWarnsWhenNoAuthMiddlewareGuardsTheAuthenticatedRoutes pins the
// warning the server starts with when routes of the authenticated group
// were registered and no auth middleware was, and its absence once one is:
// the gRPC listener refuses nothing in the same situation, so neither does
// this one.
func TestRunWarnsWhenNoAuthMiddlewareGuardsTheAuthenticatedRoutes(t *testing.T) {
	warnings := func(t *testing.T) *observer.ObservedLogs {
		t.Helper()
		core, logs := observer.New(zapcore.WarnLevel)
		restore := zap.ReplaceGlobals(zap.New(core))
		t.Cleanup(restore)
		return logs
	}
	registered := func(t *testing.T, guarded bool, endpoints ...string) {
		t.Helper()
		routeMu.Lock()
		wasRoutes, wasGuarded := authRoutes, authGuarded.Load()
		authRoutes = make(map[string]bool, len(endpoints))
		for _, endpoint := range endpoints {
			authRoutes[endpoint] = true
		}
		routeMu.Unlock()
		authGuarded.Store(guarded)
		t.Cleanup(func() {
			routeMu.Lock()
			authRoutes = wasRoutes
			routeMu.Unlock()
			authGuarded.Store(wasGuarded)
		})
	}

	t.Run("without an auth middleware", func(t *testing.T) {
		logs := warnings(t)
		registered(t, false, "/api/records", "/api/items")

		warnUnguardedRoutes(zap.S())

		entries := logs.FilterMessage(unguardedRoutesMsg).All()
		require.Len(t, entries, 1)
		require.Equal(t, []any{"/api/items", "/api/records"}, entries[0].ContextMap()["routes"])
	})

	t.Run("with one", func(t *testing.T) {
		logs := warnings(t)
		registered(t, true, "/api/records")

		warnUnguardedRoutes(zap.S())

		require.Empty(t, logs.FilterMessage(unguardedRoutesMsg).All())
	})

	t.Run("with public routes alone", func(t *testing.T) {
		logs := warnings(t)
		registered(t, false)

		warnUnguardedRoutes(zap.S())

		require.Empty(t, logs.FilterMessage(unguardedRoutesMsg).All())
	})
}
