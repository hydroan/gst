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

func Run() error {
	mu.Lock()
	defer mu.Unlock()
	if !config.App.GopsEnabled {
		return nil
	}

	if err := agent.Listen(agent.Options{
		Addr: fmt.Sprintf("%s:%d", config.App.GopsListen, config.App.GopsPort),
		// Never set ShutdownCleanup to true: gops then traps termination signals and calls os.Exit(1) itself.
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
