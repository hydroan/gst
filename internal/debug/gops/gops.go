// Package gops runs the gops agent, through which the gops command inspects
// the running process, when the configuration enables it (config.App's
// GopsEnabled): bootstrap starts it with the process and stops it as the
// process shuts down.
package gops

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/gops/agent"
	"github.com/hydroan/gst/config"
	"go.uber.org/zap"
)

var (
	running bool
	mu      sync.Mutex
)

// Run starts the gops agent listening on the configured address and returns:
// the agent serves on a goroutine of its own. With gops disabled in the
// configuration it starts nothing.
func Run() error {
	mu.Lock()
	defer mu.Unlock()
	if !config.App.GopsEnabled {
		return nil
	}

	if err := agent.Listen(agent.Options{
		Addr: fmt.Sprintf("%s:%d", config.App.GopsListen, config.App.GopsPort),
		// Never set ShutdownCleanup to true: gops then traps termination
		// signals and calls os.Exit(1) itself.
		ShutdownCleanup: false,
		ConfigDir:       filepath.Join(os.TempDir(), "gops"),
	}); err != nil {
		zap.S().Errorw("gops agent startup failed", "err", err)
		return err
	}

	running = true
	zap.S().Infow("gops agent started", "listen", config.App.GopsListen, "port", config.App.GopsPort)
	return nil
}

// Stop closes the gops agent Run started; with none running it does nothing.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if !running {
		return
	}

	zap.S().Infow("gops agent shutdown initiated")
	agent.Close()
	running = false
	zap.S().Infow("gops agent shutdown completed")
}
