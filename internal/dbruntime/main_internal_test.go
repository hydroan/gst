package dbruntime

import (
	"fmt"
	"os"
	"testing"
)

// TestMain releases the mysql and postgres containers newMySQLDB and
// newPostgresDB prepare. Only the tests covering server-dialect index
// behavior ask for one, so both start lazily and this is the only place that
// knows whether there is anything to release.
func TestMain(m *testing.M) {
	code := m.Run()
	for _, release := range []func() error{releaseMySQL, releasePostgres} {
		if release != nil {
			if err := release(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to release the test database: %v\n", err)
			}
		}
	}
	os.Exit(code)
}
