package rbac_test

import (
	"os"
	"testing"

	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/testutil/testlog"
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
	// The logs go to a directory of their own, out of the test output and the
	// package source tree; the process runs no config.Init, and ToTempDir
	// writes the settings to config.App for the logger Init to read.
	_, releaseLogs, err := testlog.ToTempDir()
	if err != nil {
		panic(err)
	}
	defer func() { _ = releaseLogs() }()
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
