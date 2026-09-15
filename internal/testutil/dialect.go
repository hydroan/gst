package testutil

import (
	"os"

	"github.com/hydroan/gst/config"
)

// envDatabaseUnderTest names the dialect a framework suite runs against. It
// is a contract between the suites' TestMain functions and the Makefile test
// target, which repeats them once per dialect. The public testutil knows
// nothing about it, so projects built on it keep full control of their own
// Server.Database.
const envDatabaseUnderTest = "GST_TEST_DATABASE"

// DatabaseUnderTest returns the dialect a framework suite runs against:
// MySQL, the framework's primary dialect, unless GST_TEST_DATABASE names
// another. An unsupported value fails the run through the Server.Database
// validation.
func DatabaseUnderTest() config.DBType {
	if override := os.Getenv(envDatabaseUnderTest); len(override) > 0 {
		return config.DBType(override)
	}
	return config.DBMySQL
}
