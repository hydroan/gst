package dbruntime

import (
	"net/url"
	"strings"

	"github.com/hydroan/gst/config"
)

// SQLiteInMemory reports whether cfg selects sqlite's in-memory database
// rather than a database file: with is_memory set, with no path, with a path
// naming memory, plainly or as a file URI, with or without parameters, or
// with a file URI in memory mode. It reports true for
// {IsMemory: true, Path: "./data.db"}, {Path: ""}, {Path: ":memory:"},
// {Path: "file::memory:?cache=shared"} and {Path: "file:data.db?mode=memory"},
// and false for {Path: "./data.db"}, {Path: "./data.db?_busy_timeout=1000"}
// and {Path: "data.db?mode=memory"}.
//
// A path selecting memory counts whatever is_memory says: the two spell the
// same intent, and honoring the path as a file would open a database that
// lives as long as one connection and disappears with it, which is not what
// either of them asked for.
func SQLiteInMemory(cfg config.Sqlite) bool {
	if cfg.IsMemory || len(cfg.Path) == 0 {
		return true
	}
	name, query, _ := strings.Cut(cfg.Path, "?")
	file, uri := strings.CutPrefix(name, "file:")
	if file == ":memory:" {
		return true
	}
	if !uri {
		return false
	}
	// sqlite takes a file URI's mode from its last mode parameter; outside a
	// URI the driver reads its own parameters alone, and mode is not one.
	// ParseQuery keeps every pair that decodes when another does not, and
	// mode=memory needs no decoding.
	params, _ := url.ParseQuery(query)
	modes := params["mode"]
	return len(modes) > 0 && modes[len(modes)-1] == "memory"
}
