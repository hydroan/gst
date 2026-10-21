package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestParseRegisteredModelsRunsOncePerOpenedDatabase pins that the parse is
// recorded once per opened database, however the handle reaching it was
// derived: gorm gives every session a configuration of its own, so a
// session, a context-bound handle, a debug handle and a transaction each
// carry a different one, while all of them share the schema cache of the
// database they came from. A record per handle would grow with every call.
func TestParseRegisteredModelsRunsOncePerOpenedDatabase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() {
		tx.Rollback()
		require.NoError(t, sqlDB.Close())
	})

	records := func() int {
		n := 0
		parsedHandles.Range(func(any, any) bool {
			n++
			return true
		})
		return n
	}
	before := records()
	for _, handle := range []*gorm.DB{db, db.Session(&gorm.Session{}), db.WithContext(context.Background()), db.Debug(), tx} {
		parseRegisteredModels(handle)
	}
	require.Equal(t, before+1, records(), "one opened database, one record")
}
