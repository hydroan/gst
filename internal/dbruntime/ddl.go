package dbruntime

import (
	"fmt"
	"strings"

	"github.com/hydroan/gst/config"
	"gorm.io/gorm"
)

// The statements the framework tells an operator to run, rendered for the
// database a gorm dialector speaks to and quoted by that dialector itself:
// the startup refusal of an index found under another name (see
// ensureCustomIndexes) and the advisory of gg migrate both render them, and
// MySQL spells them differently from the others.

// QuoteIdentifier returns ident quoted the way dialector quotes it: `records`
// on MySQL and SQLite, "records" on PostgreSQL, a quote inside the name
// doubled, and a name qualified by its schema quoted part by part,
// "analytics"."records" on PostgreSQL.
func QuoteIdentifier(dialector gorm.Dialector, ident string) string {
	var b strings.Builder
	dialector.QuoteTo(&b, ident)
	return b.String()
}

// RenameTableSQL returns the statement renaming the table from so that it
// is named to, for dialector: RENAME TABLE `old_records` TO `records`; on
// MySQL, ALTER TABLE "old_records" RENAME TO "records"; on PostgreSQL, and
// ALTER TABLE `old_records` RENAME TO `records`; on SQLite.
func RenameTableSQL(dialector gorm.Dialector, from, to string) string {
	if isMySQL(dialector) {
		return fmt.Sprintf("RENAME TABLE %s TO %s;", QuoteIdentifier(dialector, from), QuoteIdentifier(dialector, to))
	}
	return fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", QuoteIdentifier(dialector, from), QuoteIdentifier(dialector, to))
}

// RenameIndexSQL returns the statement renaming the index from of table so
// that it is named to, for dialector: ALTER TABLE `records` RENAME INDEX
// `legacy_records_kind` TO `idx_records_kind`; on MySQL, ALTER INDEX
// "legacy_records_kind" RENAME TO "idx_records_kind"; on PostgreSQL, and
// on SQLite, which renames no index, DROP INDEX `legacy_records_kind`; for
// the index to be created under its name at the next start.
func RenameIndexSQL(dialector gorm.Dialector, table, from, to string) string {
	switch {
	case isMySQL(dialector):
		return fmt.Sprintf("ALTER TABLE %s RENAME INDEX %s TO %s;", QuoteIdentifier(dialector, table), QuoteIdentifier(dialector, from), QuoteIdentifier(dialector, to))
	case strings.EqualFold(dialector.Name(), string(config.DBSqlite)):
		return fmt.Sprintf("DROP INDEX %s;", QuoteIdentifier(dialector, from))
	}
	return fmt.Sprintf("ALTER INDEX %s RENAME TO %s;", QuoteIdentifier(dialector, from), QuoteIdentifier(dialector, to))
}

// isMySQL reports whether dialector speaks to MySQL.
func isMySQL(dialector gorm.Dialector) bool {
	return strings.EqualFold(dialector.Name(), string(config.DBMySQL))
}
