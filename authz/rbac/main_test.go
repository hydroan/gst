package rbac_test

import (
	"os"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	zaplogger "github.com/hydroan/gst/logger/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestMain gives the package a database because every policy write opens a
// transaction. An in-memory SQLite one is enough: the tests that exercise only
// the in-memory set pair it with nullStorage and never write a row, so all it
// has to do for them is begin and commit.
func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests wires the loggers and the database. os.Exit in TestMain would skip
// the deferred removal of the log directory, hence the wrapper.
func runTests(m *testing.M) int {
	// Opening a transaction logs through logger.Database, and a failed in-memory
	// update logs through logger.Authz. Both are nil until the loggers are wired.
	// The process runs no config.Init, so the output is set here. File mode
	// keeps the logs out of the test output once the global stream names a
	// file of its own: naming none, it would still write to stdout. The console
	// mirror is off already, as it is in any config never initialized. A log
	// directory of its own keeps the files out of the package source tree: log
	// files written there change with every run, which go's test cache takes
	// for changed source in every test that reads or lists that tree.
	logDir, err := os.MkdirTemp("", "gst_logs_")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(logDir) }()
	config.App.Logger.Output = config.LoggerOutputFile
	config.App.Logger.Dir = logDir
	config.App.Logger.File = "global.log"
	if err = zaplogger.Init(); err != nil {
		panic(err)
	}
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{TranslateError: true})
	if err != nil {
		panic(err)
	}
	// Assigned rather than installed through dbruntime.InitDatabase, which also
	// starts the table builder. Tests needing a table create it themselves.
	dbruntime.DB = db
	return m.Run()
}
