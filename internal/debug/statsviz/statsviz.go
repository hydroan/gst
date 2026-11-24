// Package statsviz serves the statsviz page, live charts of the Go runtime's
// metrics, on a listener of its own when the configuration enables it
// (config.App's StatsvizEnabled): bootstrap starts the server with the
// process and stops it as the process shuts down.
package statsviz

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/arl/statsviz"
	"github.com/hydroan/gst/config"
	"go.uber.org/zap"
)

var server *http.Server

// Run serves the statsviz page on the configured address and returns when
// the server ends: nil once Stop has shut it down, the error that kept it
// from serving otherwise. With statsviz disabled in the configuration it
// returns nil at once.
func Run() error {
	if !config.App.StatsvizEnabled {
		return nil
	}

	mux := http.NewServeMux()
	if err := statsviz.Register(mux); err != nil {
		zap.S().Errorw("failed to register statsviz handler", "err", err)
		return err
	}
	server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.App.StatsvizListen, config.App.StatsvizPort),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		Handler:      mux,
	}

	zap.S().Infow("statsviz server started", "listen", config.App.StatsvizListen, "port", config.App.StatsvizPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		zap.S().Errorw("failed to start statsviz server", "err", err)
		return err
	}

	return nil
}

// drainTimeout bounds how long Stop waits for the requests in flight.
const drainTimeout = 5 * time.Second

// Stop shuts the statsviz server down: it stops accepting connections and
// waits for the requests in flight for up to drainTimeout and no longer than
// abandon lasts, not at all when it has already ended, for a process that
// must not wait on anything. The connections a drain cut short leaves open
// are closed.
func Stop(abandon context.Context) {
	if server == nil {
		return
	}

	zap.S().Infow("statsviz server shutdown initiated")
	ctx, cancel := context.WithTimeout(abandon, drainTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		zap.S().Warnw("statsviz server closing the connections its drain left open", "err", err, "reason", context.Cause(ctx))
		if closeErr := server.Close(); closeErr != nil {
			zap.S().Errorw("statsviz server close failed", "err", closeErr)
		}
	} else {
		zap.S().Infow("statsviz server shutdown completed")
	}
	server = nil
}
