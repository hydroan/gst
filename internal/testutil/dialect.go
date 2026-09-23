package testutil

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/hydroan/gst/config"
)

// envRetiredDatabaseUnderTest is the environment variable that selected the
// dialect under test before the build tags did. Nothing reads it for that any
// more, see DatabaseUnderTest.
const envRetiredDatabaseUnderTest = "GST_TEST_DATABASE"

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
//
// Two mistakes stop the run instead of letting the suite run against MySQL
// once more under the impression it ran against another dialect. A misspelled
// tag selects no dialect, but the binary's build settings still carry it, so a
// binary that fell back to MySQL while built with one returns what the tag
// names instead, gsttest_postgre as postgre, which the test database setup
// then rejects like every dialect it has no preparation for. The retired
// GST_TEST_DATABASE variable panics with the tag to build with instead; since
// go test replays a cached result without running TestMain at all, that stop
// comes once the suite actually runs, which it does after any change to what
// the suite tests.
func DatabaseUnderTest() config.DBType {
	if dialect := os.Getenv(envRetiredDatabaseUnderTest); len(dialect) > 0 {
		panic(fmt.Sprintf("%s is no longer read, build the test with -tags gsttest_%s instead",
			envRetiredDatabaseUnderTest, dialect))
	}
	if databaseUnderTest != config.DBMySQL {
		return databaseUnderTest
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return databaseUnderTest
	}
	for _, setting := range info.Settings {
		if setting.Key != "-tags" {
			continue
		}
		if dialect, tagged := dialectTagged(setting.Value); tagged {
			return dialect
		}
	}
	return databaseUnderTest
}

// dialectTagged returns the dialect the first gsttest_ tag of a comma
// separated build tag list names, e.g. "race,gsttest_postgre" names postgre.
// It reports false when no tag of the list starts with gsttest_.
func dialectTagged(tags string) (config.DBType, bool) {
	for tag := range strings.SplitSeq(tags, ",") {
		if dialect, found := strings.CutPrefix(tag, "gsttest_"); found {
			return config.DBType(dialect), true
		}
	}
	return "", false
}
