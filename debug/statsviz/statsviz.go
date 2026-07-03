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

func Stop() {
	if server == nil {
		return
	}

	zap.S().Infow("statsviz server shutdown initiated")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		zap.S().Errorw("statsviz server shutdown failed", "err", err)
	} else {
		zap.S().Infow("statsviz server shutdown completed")
	}
	server = nil
}
