package testutil

import (
	"github.com/hydroan/gst/config"
)

// DatabaseUnderTest returns the dialect a framework suite runs against:
// MySQL, the framework's primary dialect, unless the test binary is built
// with the gsttest_postgres or the gsttest_sqlite build tag. The tags are a
// contract between the suites' TestMain functions and the Makefile test
// target, which repeats them once per dialect. The public testutil knows
// nothing about them, so projects built on it keep full control of their own
// Server.Database.
//
// The dialect is fixed when the test binary is built, not read from the
// environment when it runs, because go's test cache only tells runs apart by
// what it sees: a build tag makes each dialect a test binary of its own, with
// a cached result of its own, while a variable TestMain reads before m.Run
// installs the test log is invisible to the cache, which then replays one
// dialect's result for every other. Building with both tags declares the
// dialect twice and fails to compile.
func DatabaseUnderTest() config.DBType {
	return databaseUnderTest
}
