//go:build !gsttest_postgres && !gsttest_sqlite

package testutil

import (
	"github.com/hydroan/gst/config"
)

// databaseUnderTest is MySQL when no build tag selects another dialect, see
// DatabaseUnderTest.
const databaseUnderTest = config.DBMySQL
