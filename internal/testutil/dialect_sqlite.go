//go:build gsttest_sqlite

package testutil

import (
	"github.com/hydroan/gst/config"
)

// databaseUnderTest is SQLite under the gsttest_sqlite build tag, see
// DatabaseUnderTest.
const databaseUnderTest = config.DBSqlite
