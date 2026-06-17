package database

import (
	"context"
	"fmt"
	"time"

	gstotel "github.com/hydroan/gst/provider/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// GormTracingPlugin is a GORM plugin that adds distributed tracing to database operations.
type GormTracingPlugin struct{}

// Name returns the plugin name.
func (p *GormTracingPlugin) Name() string {
	return "gorm:tracing"
}

// Initialize initializes the plugin.
func (p *GormTracingPlugin) Initialize(db *gorm.DB) error {
	// Register callbacks for different operations
	if err := p.registerCallbacks(db); err != nil {
		return err
	}
	return nil
}

// registerCallbacks registers tracing callbacks for GORM operations
func (p *GormTracingPlugin) registerCallbacks(db *gorm.DB) error {
	// Create operations
	if err := db.Callback().Create().Before("gorm:create").Register("tracing:create_before", p.createBefore); err != nil {
		return err
	}
	if err := db.Callback().Create().After("gorm:create").Register("tracing:create_after", p.createAfter); err != nil {
		return err
	}

	// Query operations
	if err := db.Callback().Query().Before("gorm:query").Register("tracing:query_before", p.queryBefore); err != nil {
		return err
	}
	if err := db.Callback().Query().After("gorm:query").Register("tracing:query_after", p.queryAfter); err != nil {
		return err
	}

	// Update operations
	if err := db.Callback().Update().Before("gorm:update").Register("tracing:update_before", p.updateBefore); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:update").Register("tracing:update_after", p.updateAfter); err != nil {
		return err
	}

	// Delete operations
	if err := db.Callback().Delete().Before("gorm:delete").Register("tracing:delete_before", p.deleteBefore); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("gorm:delete").Register("tracing:delete_after", p.deleteAfter); err != nil {
		return err
	}

	// Row operations
	if err := db.Callback().Row().Before("gorm:row").Register("tracing:row_before", p.rowBefore); err != nil {
		return err
	}
	if err := db.Callback().Row().After("gorm:row").Register("tracing:row_after", p.rowAfter); err != nil {
		return err
	}

	// Raw operations
	if err := db.Callback().Raw().Before("gorm:raw").Register("tracing:raw_before", p.rawBefore); err != nil {
		return err
	}
	if err := db.Callback().Raw().After("gorm:raw").Register("tracing:raw_after", p.rawAfter); err != nil {
		return err
	}

	return nil
}

// createBefore is called before create operations
func (p *GormTracingPlugin) createBefore(db *gorm.DB) {
	p.startSpan(db, "gorm.create")
}

// createAfter is called after create operations
func (p *GormTracingPlugin) createAfter(db *gorm.DB) {
	p.finishSpan(db)
}

// queryBefore is called before query operations
func (p *GormTracingPlugin) queryBefore(db *gorm.DB) {
	p.startSpan(db, "gorm.query")
}

// queryAfter is called after query operations
func (p *GormTracingPlugin) queryAfter(db *gorm.DB) {
	p.finishSpan(db)
}

// updateBefore is called before update operations
func (p *GormTracingPlugin) updateBefore(db *gorm.DB) {
	p.startSpan(db, "gorm.update")
}

// updateAfter is called after update operations
func (p *GormTracingPlugin) updateAfter(db *gorm.DB) {
	p.finishSpan(db)
}

// deleteBefore is called before delete operations
func (p *GormTracingPlugin) deleteBefore(db *gorm.DB) {
	p.startSpan(db, "gorm.delete")
}

// deleteAfter is called after delete operations
func (p *GormTracingPlugin) deleteAfter(db *gorm.DB) {
	p.finishSpan(db)
}

// rowBefore is called before row operations
func (p *GormTracingPlugin) rowBefore(db *gorm.DB) {
	p.startSpan(db, "gorm.row")
}

// rowAfter is called after row operations
func (p *GormTracingPlugin) rowAfter(db *gorm.DB) {
	p.finishSpan(db)
}

// rawBefore is called before raw operations
func (p *GormTracingPlugin) rawBefore(db *gorm.DB) {
	p.startSpan(db, "gorm.raw")
}

// rawAfter is called after raw operations
func (p *GormTracingPlugin) rawAfter(db *gorm.DB) {
	p.finishSpan(db)
}

// startSpan starts a new tracing span for the database operation
func (p *GormTracingPlugin) startSpan(db *gorm.DB, operation string) {
	ctx := db.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Start database span
	newCtx, span := startDatabaseSpan(ctx, operation, db.Statement.Table)

	if gstotel.IsSpanRecording(span) {
		// Add GORM-specific attributes
		gstotel.AddSpanTags(span, map[string]any{
			"gorm.operation": operation,
			"gorm.table":     db.Statement.Table,
		})

		db.Set("tracing:start_time", time.Now())
	}

	// Store span and context in the statement
	db.Statement.Context = newCtx
	db.Set("tracing:span", span)
}

// finishSpan finishes the tracing span and records the results
func (p *GormTracingPlugin) finishSpan(db *gorm.DB) {
	// Get span from statement
	spanValue, exists := db.Get("tracing:span")
	if !exists {
		return
	}

	span, ok := spanValue.(trace.Span)
	if !ok {
		return
	}

	defer span.End()
	if !gstotel.IsSpanRecording(span) {
		return
	}

	// Get start time
	startTimeValue, exists := db.Get("tracing:start_time")
	if exists {
		if startTime, ok := startTimeValue.(time.Time); ok {
			duration := time.Since(startTime)
			gstotel.AddSpanTags(span, map[string]any{
				"gorm.duration_ms": duration.Milliseconds(),
			})
		}
	}

	// Add SQL information if available
	if db.Statement.SQL.String() != "" {
		gstotel.AddSpanTags(span, map[string]any{
			"gorm.sql": db.Statement.SQL.String(),
		})
	}

	// Add affected rows count
	if db.Statement.RowsAffected >= 0 {
		gstotel.AddSpanTags(span, map[string]any{
			"gorm.rows_affected": db.Statement.RowsAffected,
		})
	}

	// Record error if any
	if db.Error != nil {
		gstotel.RecordError(span, db.Error)
		gstotel.AddSpanTags(span, map[string]any{
			"gorm.error": db.Error.Error(),
		})
	}

	// Add database connection info
	if db.Statement.ConnPool != nil {
		gstotel.AddSpanTags(span, map[string]any{
			"gorm.connection_pool": fmt.Sprintf("%T", db.Statement.ConnPool),
		})
	}
}

// InstallGormTracingPlugin installs the GORM tracing plugin to the given database instance.
func InstallGormTracingPlugin(db *gorm.DB) error {
	plugin := &GormTracingPlugin{}
	return db.Use(plugin)
}

// startDatabaseSpan starts a span for database operations
func startDatabaseSpan(ctx context.Context, operation, table string) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("db.%s %s", operation, table)
	ctx, span := gstotel.StartSpan(ctx, spanName)

	if gstotel.IsSpanRecording(span) {
		// Add database-specific attributes
		span.SetAttributes(
			attribute.String("db.operation", operation),
			attribute.String("db.table", table),
			attribute.String("component", "database"),
		)
	}

	return ctx, span
}
