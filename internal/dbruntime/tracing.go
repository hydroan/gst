package dbruntime

import (
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// InstallTracing installs the GORM OpenTelemetry plugin on a freshly opened
// handle, the one step every dialect's New shares: from then on each
// statement the handle runs exports a span parented on the statement's
// context, which is how SQL spans nest under the framework's database
// operation spans. A failure only warns: tracing is an observability aid,
// and a handle that cannot be traced still serves its statements.
func InstallTracing(db *gorm.DB) {
	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		zap.S().Warnw("failed to install GORM OpenTelemetry tracing plugin", "dialect", db.Dialector.Name(), "error", err)
	}
}
