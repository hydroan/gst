package dbruntime

import (
	"fmt"
	"strings"

	"github.com/hydroan/gst/config"
)

// The statements the framework tells an operator to run, rendered for the
// dialect the handle runs on, named the way gorm's dialectors name
// themselves: the startup refusal of an index found under another name
// (see ensureCustomIndexes) and the advisory of gg migrate both quote them,
// and MySQL spells them differently from the others.

// QuoteIdentifier returns ident quoted the way the dialect named dialect
// reads an identifier: `records` on MySQL, "records" on PostgreSQL and
// SQLite.
func QuoteIdentifier(dialect, ident string) string {
	if isMySQL(dialect) {
		return "`" + ident + "`"
	}
	return `"` + ident + `"`
}

// RenameTableSQL returns the statement renaming the table from so that it
// is named to, on dialect: RENAME TABLE `old_records` TO `records`; on
// MySQL, ALTER TABLE "old_records" RENAME TO "records"; on PostgreSQL and
// SQLite.
func RenameTableSQL(dialect, from, to string) string {
	if isMySQL(dialect) {
		return fmt.Sprintf("RENAME TABLE %s TO %s;", QuoteIdentifier(dialect, from), QuoteIdentifier(dialect, to))
	}
	return fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", QuoteIdentifier(dialect, from), QuoteIdentifier(dialect, to))
}

// RenameIndexSQL returns the statement renaming the index from of table so
// that it is named to, on dialect: ALTER TABLE `records` RENAME INDEX
// `legacy_records_kind` TO `idx_records_kind`; on MySQL, ALTER INDEX
// "legacy_records_kind" RENAME TO "idx_records_kind"; on PostgreSQL, and
// on SQLite, which renames no index, DROP INDEX "legacy_records_kind"; for
// the index to be created under its name at the next start.
func RenameIndexSQL(dialect, table, from, to string) string {
	switch {
	case isMySQL(dialect):
		return fmt.Sprintf("ALTER TABLE %s RENAME INDEX %s TO %s;", QuoteIdentifier(dialect, table), QuoteIdentifier(dialect, from), QuoteIdentifier(dialect, to))
	case strings.EqualFold(dialect, string(config.DBSqlite)):
		return fmt.Sprintf("DROP INDEX %s;", QuoteIdentifier(dialect, from))
	}
	return fmt.Sprintf("ALTER INDEX %s RENAME TO %s;", QuoteIdentifier(dialect, from), QuoteIdentifier(dialect, to))
}

// isMySQL reports whether dialect names MySQL.
func isMySQL(dialect string) bool {
	return strings.EqualFold(dialect, string(config.DBMySQL))
}
