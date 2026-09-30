package database_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/stretchr/testify/require"
)

// TestWritesAnswerTheConstraintsTheClientsDataBreaks pins the sentinels a
// write answers when the data breaks a constraint of the table, on every
// dialect that enforces it: a value too long for its column, a value the
// table's check refuses, and a foreign key no record satisfies. Sqlite
// declares no length for a column, so it answers no value too long.
func TestWritesAnswerTheConstraintsTheClientsDataBreaks(t *testing.T) {
	ctx := context.Background()
	sqlite := config.App.Database.Type == config.DBSqlite

	t.Run("a value too long for its column", func(t *testing.T) {
		if sqlite {
			t.Skip("sqlite stores a value of any length")
		}
		err := database.Database[*TestConstrainedRecord](ctx).Create(&TestConstrainedRecord{Code: "toolong", Level: 3})
		require.ErrorIs(t, err, database.ErrValueTooLong)
	})

	t.Run("a value the table's check refuses", func(t *testing.T) {
		err := database.Database[*TestConstrainedRecord](ctx).Create(&TestConstrainedRecord{Code: "ok", Level: 9})
		require.ErrorIs(t, err, database.ErrCheckConstraintViolated)
	})

	t.Run("a foreign key no record satisfies", func(t *testing.T) {
		err := database.Database[*TestConstrainedEntry](ctx).Create(&TestConstrainedEntry{RecordID: "no-such-record"})
		require.ErrorIs(t, err, database.ErrForeignKeyViolated)
	})
}
