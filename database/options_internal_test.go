package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

// TestWithTxGraftsTransactionSpanOntoContext verifies that WithTx lifts the span carried by
// the transaction handle's statement context (bound there by Transaction/TransactionFunc)
// onto the caller's operation context, so spans of operations joining the transaction nest
// under the transaction span while all values of the caller's own context are preserved.
func TestWithTxGraftsTransactionSpanOntoContext(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: glogger.Default.LogMode(glogger.Silent),
	})
	require.NoError(t, err)

	traceID, err := trace.TraceIDFromHex("11111111111111111111111111111111")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("2222222222222222")
	require.NoError(t, err)
	txSpanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	txCtx := trace.ContextWithSpanContext(context.Background(), txSpanContext)

	type callerCtxKey struct{}
	db := &database[*syncBenchPlainItem]{
		ins: gormDB,
		ctx: context.WithValue(context.Background(), callerCtxKey{}, "caller-value"),
	}
	db.WithTx(gormDB.WithContext(txCtx))

	grafted := trace.SpanFromContext(db.ctx).SpanContext()
	require.True(t, grafted.IsValid(), "WithTx should graft the transaction span onto the caller context")
	require.Equal(t, traceID, grafted.TraceID())
	require.Equal(t, spanID, grafted.SpanID())
	require.Equal(t, "caller-value", db.ctx.Value(callerCtxKey{}), "caller context values must be preserved")

	tx, ok := txFromContext(db.ctx)
	require.True(t, ok, "transaction must still be stored in the context")
	require.NotNil(t, tx)
}
