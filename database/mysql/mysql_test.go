package mysql_test

import (
	"sync"
	"testing"

	gstmysql "github.com/hydroan/gst/database/mysql"
	"github.com/stretchr/testify/require"
	gormschema "gorm.io/gorm/schema"
)

// TestDryRunAnswersColumnTypesWithoutAServer pins what DryRun is for: a
// handle on the MySQL dialect that never connects and still answers the
// column type a field gets, longtext for a string field without a size.
func TestDryRunAnswersColumnTypesWithoutAServer(t *testing.T) {
	db, err := gstmysql.DryRun()
	require.NoError(t, err)
	require.True(t, db.DryRun)
	require.Equal(t, "mysql", db.Dialector.Name())

	type sample struct {
		Owner string
	}
	parsed, err := gormschema.Parse(&sample{}, &sync.Map{}, db.NamingStrategy)
	require.NoError(t, err)
	require.Equal(t, "longtext", db.Migrator().FullDataTypeOf(parsed.FieldsByName["Owner"]).SQL)
}
