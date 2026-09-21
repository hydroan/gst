package column

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// sampleColumns are the columns a table would be registered with; they are
// both what the endpoint answers with and the only filters it accepts.
var sampleColumns = []string{"kind", "region"}

func TestQueryColumnsReadsDistinctValues(t *testing.T) {
	db := sampleTable(t)

	values, err := queryColumnsWithQuery("samples", sampleColumns, nil, db)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"alpha", "beta", "al'pha"}, values["kind"],
		"a soft deleted row, a NULL and an empty value are no filter options")
	require.ElementsMatch(t, []string{"east", "west"}, values["region"])

	values, err = queryColumnsWithQuery("samples", sampleColumns, map[string][]string{"region": {"east"}}, db)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha"}, values["kind"], "the filter narrows the other columns too")
	require.Equal(t, []string{"east"}, values["region"])

	values, err = queryColumnsWithQuery("samples", sampleColumns, map[string][]string{"kind": {"alpha,beta"}}, db)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"alpha", "beta"}, values["kind"], "a comma separated list means any of them")

	values, err = queryColumnsWithQuery("samples", sampleColumns, map[string][]string{"_page": {"2"}}, db)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"alpha", "beta", "al'pha"}, values["kind"], "a framework parameter is no column filter")
}

func TestQueryColumnsBindsFilterValues(t *testing.T) {
	db := sampleTable(t)

	// A value carrying a quote is matched as that value: it binds as a
	// statement parameter rather than being spliced into the statement, where
	// the quote would end the literal and break the statement.
	values, err := queryColumnsWithQuery("samples", sampleColumns, map[string][]string{"kind": {"al'pha"}}, db)
	require.NoError(t, err)
	require.Equal(t, []string{"al'pha"}, values["kind"])
	require.Equal(t, []string{"west"}, values["region"], "the quoted value narrows the other columns too")

	// A value written to read as SQL matches no row, and breaks nothing.
	for _, value := range []string{"alpha' OR 'a'='a", "alpha') OR ('a'='a"} {
		values, err := queryColumnsWithQuery("samples", sampleColumns, map[string][]string{"kind": {value}}, db)
		require.NoError(t, err, "value %q must not break the statement", value)
		require.Empty(t, values["kind"], "value %q must match no row", value)
		require.Empty(t, values["region"])
	}
}

func TestQueryColumnsRefusesUnregisteredFilter(t *testing.T) {
	db := sampleTable(t)

	// A parameter naming no registered column is refused, whether it is a
	// typo or a name written to carry SQL of its own.
	_, err := queryColumnsWithQuery("samples", sampleColumns, map[string][]string{"nosuch": {"east"}}, db)
	require.ErrorContains(t, err, `unsupported query parameter "nosuch"`)

	_, err = queryColumnsWithQuery("samples", sampleColumns, map[string][]string{"kind` IS NOT NULL OR `kind": {"zzz"}}, db)
	require.ErrorContains(t, err, "unsupported query parameter")
}

func TestQueryColumnsRendersOneStatementPerColumn(t *testing.T) {
	db := sampleTable(t)

	var (
		mu         sync.Mutex
		statements []string
	)
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:capture", func(tx *gorm.DB) {
		mu.Lock()
		defer mu.Unlock()
		statements = append(statements, tx.Statement.SQL.String())
	}))

	query := map[string][]string{"kind": {"alpha"}, "region": {"east"}}
	for range 20 {
		_, err := queryColumnsWithQuery("samples", sampleColumns, query, db)
		require.NoError(t, err)
	}

	rendered := make(map[string]struct{}, 2)
	for _, statement := range statements {
		rendered[statement] = struct{}{}
	}
	require.Len(t, rendered, len(sampleColumns),
		"one request renders one statement per column, whatever order the filters arrived in")
	for statement := range rendered {
		require.Regexp(t, "kind.+region", statement, "the filters render in column order")
	}
}

// sampleTable seeds the rows every case in this file reads: a few values per
// column, one of them carrying a quote, plus the three rows a filter option
// must never be built from.
func sampleTable(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "column.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE samples (kind TEXT, region TEXT, deleted_at DATETIME)").Error)
	require.NoError(t, db.Exec(`INSERT INTO samples (kind, region, deleted_at) VALUES
		('alpha', 'east', NULL),
		('beta', 'west', NULL),
		('al''pha', 'west', NULL),
		('', 'west', NULL),
		(NULL, 'west', NULL),
		('gone', 'east', '2026-01-01 00:00:00')`).Error)
	return db
}
