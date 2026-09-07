package dbruntime

import (
	"github.com/hydroan/gst/config"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// InstallTracing instruments a freshly opened handle for tracing when tracing
// is configured on, the one step every dialect's New shares: from then on
// each statement the handle runs exports a span parented on the statement's
// context, which is how SQL spans nest under the framework's database
// operation spans. A failure only warns: tracing is an observability aid,
// and a handle that cannot be traced still serves its statements.
//
// With tracing configured off the handle is left bare. The plugin would
// otherwise run on every statement for nobody: with no tracer provider ever
// installed it still wraps the statement context twice and boxes a
// non-recording span, six allocations per statement measured on a List.
//
// The decision reads the configuration rather than otel.IsEnabled: the
// default handles open during bootstrap before the otel package initializes,
// when IsEnabled is still false for a deployment that has tracing on, and
// the configured flag is the very fact that initialization consults first.
// The plugin binds its tracer when installed, so a handle opened with
// tracing off stays untraced whatever the process does afterwards.
func InstallTracing(db *gorm.DB) {
	if !config.App.OTEL.Enabled {
		return
	}
	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		zap.S().Warnw("failed to install GORM OpenTelemetry tracing plugin", "dialect", db.Dialector.Name(), "error", err)
	}
}
