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
	// Opening a transaction logs through logger.Database, and a failed in-memory
	// update logs through logger.Authz. Both are nil until the loggers are wired.
	// The process runs no config.Init, so the output is set here: file mode
	// keeps the logs out of the test output.
	config.App.Logger.Output = config.LoggerOutputFile
	if err := zaplogger.Init(); err != nil {
		panic(err)
	}
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{TranslateError: true})
	if err != nil {
		panic(err)
	}
	// Assigned rather than installed through dbruntime.InitDatabase, which also
	// starts the table builder. Tests needing a table create it themselves.
	dbruntime.DB = db
	os.Exit(m.Run())
}
