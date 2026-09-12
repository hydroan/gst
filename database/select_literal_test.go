package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// Tests for the constant side of the select builder (select_literal.go): how a
// Literal renders, what it may hold, and where it may sit.

func TestSelectLiteral(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)
	ctx := context.Background()

	type row struct {
		Source string
		Total  int64
	}

	t.Run("ProjectsAConstantColumn", func(t *testing.T) {
		one := row{}
		require.NoError(t, database.Select[*TestAggregateRecord, row](ctx,
			types.Literal("records").As("source"), TestAggregateRecordCols.Amount.Sum().As("total")).
			ScanOne(&one))
		require.Equal(t, row{Source: "records", Total: 2100}, one)

		statements := make([]types.SQLStatement, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, row](ctx,
			types.Literal("records").As("source"), TestAggregateRecordCols.Amount.Sum().As("total")).
			WithDryRun(&statements).ScanOne(&one))
		require.Len(t, statements, 1)
		require.Contains(t, statements[0].Query, "SELECT 'records' AS "+quoteIdent("source")+", COALESCE(SUM(",
			"the constant is inlined as a string literal, never bound")
		require.Empty(t, statements[0].Args)
	})

	t.Run("StaysOutOfGroupBy", func(t *testing.T) {
		type grouped struct {
			Source   string
			Category string
			Total    int64
		}
		statements := make([]types.SQLStatement, 0)
		rows := make([]grouped, 0)
		require.NoError(t, database.Select[*TestAggregateRecord, grouped](ctx,
			types.Literal("records").As("source"), TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
			WithDryRun(&statements).Scan(&rows))
		require.Len(t, statements, 1)
		require.True(t, strings.HasSuffix(statements[0].Query, "GROUP BY "+quoteIdent("category")),
			"a constant is neither a key nor a measure, so GROUP BY names the key alone")
	})

	t.Run("RejectsAValueThatIsNotAnIdentifier", func(t *testing.T) {
		one := row{}
		for _, value := range []string{"", "it's", "two words", "1st", "a-b"} {
			require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx,
				types.Literal(value).As("source"), TestAggregateRecordCols.Amount.Sum().As("total")).
				ScanOne(&one), database.ErrInvalidLiteral, "value %q", value)
		}
	})

	t.Run("RejectsALiteralWithoutAlias", func(t *testing.T) {
		one := row{}
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx,
			types.Literal("records"), TestAggregateRecordCols.Amount.Sum().As("total")).
			ScanOne(&one), database.ErrLiteralWithoutAlias)
	})

	t.Run("RejectsAWindowedLiteral", func(t *testing.T) {
		rows := make([]row, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, row](ctx,
			types.Literal("records").As("source").Over(types.OrderBy(TestAggregateRecordCols.ID.Asc())),
			TestAggregateRecordCols.Amount.Sum().As("total")).
			Scan(&rows), database.ErrWindowOnKey)
	})

	t.Run("AloneItIsAPlainRead", func(t *testing.T) {
		type only struct {
			Source string
		}
		rows := make([]only, 0)
		require.ErrorIs(t, database.Select[*TestAggregateRecord, only](ctx, types.Literal("records").As("source")).
			Scan(&rows), database.ErrPlainSelect)
	})
}
