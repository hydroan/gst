package testcontainer

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
)

const sqliteDatabase = "test"

// setupSqlite prepares a file backed sqlite database in a directory of its own
// and points the framework at it. The returned function removes that directory.
//
// Sqlite needs no container, but a file still gives each call the isolation a
// container gives: the framework default is an in-memory database shared by
// every connection in the process.
func setupSqlite() (func() error, error) {
	dir, err := os.MkdirTemp("", "gst_sqlite_")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create sqlite directory")
	}
	path := filepath.Join(dir, sqliteDatabase+".db")

	applyConfigToEnv(config.Sqlite{
		Path:     path,
		Database: sqliteDatabase,
	})
	// File mode is the zero value of IsMemory and applyConfigToEnv skips zero
	// values, while the framework defaults the field to true.
	os.Setenv(config.SQLITE_IS_MEMORY, "false")
	useDatabase(config.DBSqlite)
	reportServiceReady(string(config.DBSqlite), path)

	return func() error { return os.RemoveAll(dir) }, nil
}
