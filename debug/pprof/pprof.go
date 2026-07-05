// Package debugpprof provides an HTTP server for pprof endpoints; the package name
// avoids conflicting with the standard library "pprof" packages.
package debugpprof

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // #nosec G108 -- pprof endpoint is intentionally exposed for debugging purposes
	"runtime"
	"time"

	"github.com/hydroan/gst/config"
	"go.uber.org/zap"
)

var server *http.Server

func Run() error {
	if !config.App.PprofEnabled {
		return nil
	}
	// A fraction/rate of 1 records every single mutex contention and blocking
	// event (with its stack trace), which keeps sampling continuously even
	// when nobody is querying /debug/pprof/*. Under real production
	// concurrency that adds a persistent CPU cost unrelated to actual pprof
	// usage, so sample instead of recording exhaustively: report 1 in 100
	// contended mutex events, and only stack-trace blocking events that last
	// at least 100us.
	runtime.SetMutexProfileFraction(100)
	runtime.SetBlockProfileRate(100_000)

	server = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", config.App.PprofListen, config.App.PprofPort),
		Handler:           http.DefaultServeMux,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second, // Prevent Slowloris attacks
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		zap.S().Errorw("failed to start pprof server", "err", err)
		return err
	}
	zap.S().Infow("pprof server started", "listen", config.App.PprofListen, "port", config.App.PprofPort)

	return nil
}

func Stop() {
	if server == nil {
		return
	}

	zap.S().Infow("pprof server shutdown initiated")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		zap.S().Errorw("pprof server shutdown failed", "err", err)
	} else {
		zap.S().Infow("pprof server shutdown completed")
	}
	server = nil
}
