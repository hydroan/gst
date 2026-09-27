package component

import (
	"context"
	"runtime"
	"time"

	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// reportRuntime logs the process's goroutines and heap every minute, until
// the process shuts down: figures of this replica alone, which is what a
// component is for.
func reportRuntime(ctx context.Context) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		logger.App.Infoz("runtime report", zap.Int("goroutines", runtime.NumGoroutine()), zap.Uint64("heap_bytes", stats.HeapAlloc))
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
