package testutil

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestDatabaseUnderTestStopsOnTheRetiredVariable(t *testing.T) {
	t.Setenv(envRetiredDatabaseUnderTest, "postgres")

	require.PanicsWithValue(t,
		"GST_TEST_DATABASE is no longer read, build the test with -tags gsttest_postgres instead",
		func() { DatabaseUnderTest() })
}

func TestDialectTagged(t *testing.T) {
	tests := []struct {
		name    string
		tags    string
		dialect config.DBType
		tagged  bool
	}{
		{name: "the only tag", tags: "gsttest_postgres", dialect: config.DBPostgres, tagged: true},
		{name: "among other tags", tags: "netgo,gsttest_sqlite", dialect: config.DBSqlite, tagged: true},
		// A misspelled dialect is named as it is, for the test database setup
		// to reject.
		{name: "misspelled dialect", tags: "gsttest_postgre", dialect: "postgre", tagged: true},
		{name: "no dialect tag", tags: "netgo,osusergo"},
		{name: "no tags", tags: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialect, tagged := dialectTagged(tt.tags)
			require.Equal(t, tt.dialect, dialect)
			require.Equal(t, tt.tagged, tagged)
		})
	}
}
