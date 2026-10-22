package dbmigrate

import (
	"fmt"
	"strings"

	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/sqldef/sqldef/v3/schema"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// dialectorFor returns the gorm dialector of the database the generator mode
// targets, connected to nothing: dbruntime quotes the identifiers and spells
// the rename statements of the advisory with it, the way gorm speaks to that
// server, so the executable block pastes into it as it is.
func dialectorFor(mode schema.GeneratorMode) gorm.Dialector {
	switch mode {
	case schema.GeneratorModePostgres:
		return postgres.New(postgres.Config{})
	case schema.GeneratorModeSQLite3:
		return sqlite.New(sqlite.Config{})
	}
	return mysql.New(mysql.Config{})
}

// combineAdvisories joins the non-empty advisory sections with one blank
// line. Each section already ends with a newline of its own.
func combineAdvisories(sections ...string) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		if len(section) > 0 {
			parts = append(parts, section)
		}
	}
	return strings.Join(parts, "\n")
}

// formatTableRenames renders the advisory body shown after a migration plan
// whose DROP TABLE / CREATE TABLE pairs were verified as renames. The caller
// owns the surrounding section title and placement; executing the statements
// stays a human decision.
//
// Output rules keep copy-paste safe: every explanatory line carries a "--"
// SQL comment prefix so pasting the whole block into the server stays
// harmless, and only directly executable statements appear unprefixed,
// grouped at the end after a blank line. Each table rename precedes the
// index renames of its own table, which only apply once the table is
// renamed. Remaining changes render as comments only: they belong to the
// next migration run, not to the rename. On SQLite, which renames no index,
// the index statements drop the old names for the next run to create the
// new ones, and a comment line says so.
func formatTableRenames(mode schema.GeneratorMode, pairs []tableRenamePair) string {
	if len(pairs) == 0 {
		return ""
	}
	dialector := dialectorFor(mode)

	var b strings.Builder
	b.WriteString("  -- The plan above drops and re-creates the tables below; the created table keeps every column of the dropped one, so these are renames.\n")
	b.WriteString("  -- DROP TABLE discards every row; renaming only modifies metadata and keeps the data.\n")
	b.WriteString("  -- Run the statement(s) below instead, then re-run gg migrate.\n")
	for _, pair := range pairs {
		if len(pair.Residual) > 0 {
			b.WriteString("  -- Changes marked \"remaining\" are not covered by the rename; re-run gg migrate after renaming to apply them as in-place ALTERs.\n")
			break
		}
	}
	if mode == schema.GeneratorModeSQLite3 {
		for _, pair := range pairs {
			if len(pair.IndexRenames) > 0 {
				b.WriteString("  -- SQLite renames no index: the index statements below drop the old names, and the next gg migrate creates the indexes under their new names.\n")
				break
			}
		}
	}
	for _, pair := range pairs {
		fmt.Fprintf(&b, "  -- Table %s -> %s\n", dbruntime.QuoteIdentifier(dialector, pair.From), dbruntime.QuoteIdentifier(dialector, pair.To))
		for _, index := range pair.IndexRenames {
			unique := ""
			if index.Unique {
				unique = ", UNIQUE"
			}
			fmt.Fprintf(&b, "  --   index %s -> %s (%s%s)\n", dbruntime.QuoteIdentifier(dialector, index.From), dbruntime.QuoteIdentifier(dialector, index.To), index.Columns, unique)
		}
		for _, statement := range pair.Residual {
			fmt.Fprintf(&b, "  --   remaining change: %s\n", statement)
		}
	}
	b.WriteString("\n")
	for _, pair := range pairs {
		b.WriteString(dbruntime.RenameTableSQL(dialector, pair.From, pair.To) + "\n")
		for _, index := range pair.IndexRenames {
			b.WriteString(dbruntime.RenameIndexSQL(dialector, index.Table, index.From, index.To) + "\n")
		}
	}
	return b.String()
}

// formatIndexRenames renders the advisory body shown after a migration plan
// whose drop/add pairs were verified as renames. The caller owns the
// surrounding section title and placement; executing the statements stays a
// human decision.
//
// Output rules keep copy-paste safe: every explanatory line carries a "--"
// SQL comment prefix so pasting the whole block into the server stays
// harmless, and only directly executable rename statements appear
// unprefixed, grouped at the end after a blank line.
//
// SQLite renames no index: dropping the old one and creating the new one,
// which is what the plan already holds, is the only way to the new name, so
// the plan stands and no advisory is rendered.
func formatIndexRenames(mode schema.GeneratorMode, pairs []indexRenamePair) string {
	if len(pairs) == 0 || mode == schema.GeneratorModeSQLite3 {
		return ""
	}
	dialector := dialectorFor(mode)

	var b strings.Builder
	b.WriteString("  -- The plan above drops and re-creates the indexes below with identical definitions; these are renames.\n")
	b.WriteString("  -- Renaming an index only modifies metadata; DROP + ADD rebuilds it (full table scan on large tables).\n")
	b.WriteString("  -- Run the statement(s) below instead, then re-run gg migrate.\n")
	for _, pair := range pairs {
		unique := ""
		if pair.Unique {
			unique = ", UNIQUE"
		}
		fmt.Fprintf(&b, "  -- Table %s: %s -> %s (%s%s)\n", dbruntime.QuoteIdentifier(dialector, pair.Table), dbruntime.QuoteIdentifier(dialector, pair.From), dbruntime.QuoteIdentifier(dialector, pair.To), pair.Columns, unique)
	}
	b.WriteString("\n")
	for _, pair := range pairs {
		b.WriteString(dbruntime.RenameIndexSQL(dialector, pair.Table, pair.From, pair.To) + "\n")
	}
	return b.String()
}
