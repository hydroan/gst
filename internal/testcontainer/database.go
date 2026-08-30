package testcontainer

import (
	"os"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
)

// SetupDatabase prepares the database that dbType names and points the
// framework at it, returning the function that releases it and the one to run
// once the framework has migrated it, see prepareSchemaTemplate. An empty
// dbType selects the framework default.
//
// A database the framework can connect to but that has no preparation here is
// rejected rather than quietly replaced, so a test never runs against a
// different database than the one it asked for.
func SetupDatabase(dbType config.DBType) (release func() error, afterMigrate func(), err error) {
	if len(dbType) == 0 {
		dbType = config.DBSqlite
	}

	switch dbType {
	case config.DBMySQL:
		return setupMySQL()
	case config.DBPostgres:
		release, err = setupPostgres()
		return release, noSchemaTemplatePublish, err
	case config.DBSqlite:
		release, err = setupSqlite()
		return release, noSchemaTemplatePublish, err
	default:
		return nil, nil, errors.Newf("no test database available for %q, supported are %q, %q and %q",
			dbType, config.DBMySQL, config.DBPostgres, config.DBSqlite)
	}
}

// useDatabase points the framework at dbType and turns on automatic table
// migration. database.auto_migrate defaults to false, and a freshly prepared
// database holds no tables at all, so a bootstrap needs it to create the ones
// its models declare.
func useDatabase(dbType config.DBType) {
	os.Setenv(config.DATABASE_TYPE, string(dbType))
	os.Setenv(config.DATABASE_AUTO_MIGRATE, "true")
}
