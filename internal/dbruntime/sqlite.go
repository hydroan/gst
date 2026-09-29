package dbruntime

import (
	"strings"

	"github.com/hydroan/gst/config"
)

// SqliteInMemory reports whether cfg selects sqlite's in-memory database
// rather than a database file: with is_memory set, with no path, or with a
// path naming memory, plainly or as a file URI, with or without parameters.
// It reports true for {IsMemory: true, Path: "./data.db"}, {Path: ""},
// {Path: ":memory:"} and {Path: "file::memory:?cache=shared"}, and false for
// {Path: "./data.db"} and {Path: "./data.db?_busy_timeout=1000"}.
//
// A path naming memory counts whatever is_memory says: the two spell the same
// intent, and honoring the path as a file would open a database that lives as
// long as one connection and disappears with it, which is not what either of
// them asked for.
func SqliteInMemory(cfg config.Sqlite) bool {
	if cfg.IsMemory || len(cfg.Path) == 0 {
		return true
	}
	name, _, _ := strings.Cut(cfg.Path, "?")
	return strings.TrimPrefix(name, "file:") == ":memory:"
}
