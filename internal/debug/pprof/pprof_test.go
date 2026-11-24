package debugpprof_test

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	debugpprof "github.com/hydroan/gst/internal/debug/pprof"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestRunLogsTheServerStartedWhileItServes pins when the entry announcing
// the server is written: with Run serving, before anything stops the server,
// so the log of a running process says the endpoints are up.
func TestRunLogsTheServerStartedWhileItServes(t *testing.T) {
	original := config.App
	config.App = new(config.Config)
	config.App.PprofEnabled = true
	config.App.PprofListen = "127.0.0.1"
	// Port 0 takes any free port: nothing here dials the server.
	config.App.PprofPort = 0
	t.Cleanup(func() { config.App = original })

	core, logs := observer.New(zap.InfoLevel)
	t.Cleanup(zap.ReplaceGlobals(zap.New(core)))

	served := make(chan error, 1)
	go func() { served <- debugpprof.Run() }()
	t.Cleanup(func() { debugpprof.Stop(context.Background()) })

	require.Eventually(t, func() bool {
		return logs.FilterMessage("pprof server started").Len() == 1
	}, 2*time.Second, 10*time.Millisecond, "the entry is written while the server serves")
	select {
	case err := <-served:
		t.Fatalf("Run returned before anything stopped the server: %v", err)
	default:
	}

	debugpprof.Stop(context.Background())
	select {
	case err := <-served:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return once the server was stopped")
	}
}
