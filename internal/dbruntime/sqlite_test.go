package dbruntime_test

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/stretchr/testify/require"
)

// TestSqliteInMemory pins which configurations select the in-memory database
// and which a database file, with the examples of SqliteInMemory's comment
// among them. Only the exact name counts as memory: a file of that name in a
// directory is a file.
func TestSqliteInMemory(t *testing.T) {
	for name, cfg := range map[string]config.Sqlite{
		"the_flag_over_a_path":                     {IsMemory: true, Path: "./data.db"},
		"no_path":                                  {},
		"a_path_naming_memory":                     {Path: ":memory:"},
		"a_path_naming_memory_with_parameters":     {Path: ":memory:?cache=shared"},
		"a_file_uri_naming_memory":                 {Path: "file::memory:"},
		"a_file_uri_naming_memory_with_parameters": {Path: "file::memory:?cache=shared"},
	} {
		t.Run("memory_for_"+name, func(t *testing.T) {
			require.True(t, dbruntime.SqliteInMemory(cfg))
		})
	}

	for name, cfg := range map[string]config.Sqlite{
		"a_file_path":                        {Path: "./data.db"},
		"a_file_path_with_parameters":        {Path: "./data.db?_busy_timeout=1000"},
		"a_file_uri":                         {Path: "file:data.db?cache=shared"},
		"a_file_named_memory_in_a_directory": {Path: "/tmp/:memory:"},
	} {
		t.Run("file_for_"+name, func(t *testing.T) {
			require.False(t, dbruntime.SqliteInMemory(cfg))
		})
	}
}
