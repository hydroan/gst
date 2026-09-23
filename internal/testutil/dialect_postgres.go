//go:build gsttest_postgres

package testutil

import (
	"github.com/hydroan/gst/config"
)

// databaseUnderTest is PostgreSQL under the gsttest_postgres build tag, see
// DatabaseUnderTest.
const databaseUnderTest = config.DBPostgres
