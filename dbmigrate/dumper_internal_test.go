package dbmigrate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSingleQuoteSQLiteDefaults pins the rewrite of GORM's double-quoted
// SQLite string defaults into the single-quoted literals the sqldef parser
// reads: every quoted default in a statement, an escaped double quote inside
// one, a single quote inside one, and nothing else.
func TestSingleQuoteSQLiteDefaults(t *testing.T) {
	require.Equal(t,
		"CREATE TABLE `samples` (`status` varchar(20) DEFAULT 'active',`kind` text DEFAULT 'a ''b'' c',`count` integer DEFAULT 0)",
		singleQuoteSQLiteDefaults("CREATE TABLE `samples` (`status` varchar(20) DEFAULT \"active\",`kind` text DEFAULT \"a 'b' c\",`count` integer DEFAULT 0)"))
	require.Equal(t, "DEFAULT 'say \"hi\"'", singleQuoteSQLiteDefaults(`DEFAULT "say ""hi"""`))
	require.Equal(t, "DEFAULT 'already'", singleQuoteSQLiteDefaults("DEFAULT 'already'"))
}
