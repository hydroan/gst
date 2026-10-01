package dbmigrate

import (
	"fmt"
	"strings"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/sqldef/sqldef/v3/schema"
)

// dialectFor returns the name of the dialect the generator mode targets,
// the one dbruntime renders the identifiers and rename statements of the
// advisory for, so the executable block pastes into that server as it is.
// Only the modes migrate runs the detectors for, MySQL and PostgreSQL, get
// here.
func dialectFor(mode schema.GeneratorMode) string {
	if mode == schema.GeneratorModePostgres {
		return string(config.DBPostgres)
	}
	return string(config.DBMySQL)
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
// next migration run, not to the rename.
func formatTableRenames(mode schema.GeneratorMode, pairs []tableRenamePair) string {
	if len(pairs) == 0 {
		return ""
	}
	dialect := dialectFor(mode)

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
	for _, pair := range pairs {
		fmt.Fprintf(&b, "  -- Table %s -> %s\n", dbruntime.QuoteIdentifier(dialect, pair.From), dbruntime.QuoteIdentifier(dialect, pair.To))
		for _, index := range pair.IndexRenames {
			unique := ""
			if index.Unique {
				unique = ", UNIQUE"
			}
			fmt.Fprintf(&b, "  --   index %s -> %s (%s%s)\n", dbruntime.QuoteIdentifier(dialect, index.From), dbruntime.QuoteIdentifier(dialect, index.To), index.Columns, unique)
		}
		for _, statement := range pair.Residual {
			fmt.Fprintf(&b, "  --   remaining change: %s\n", statement)
		}
	}
	b.WriteString("\n")
	for _, pair := range pairs {
		b.WriteString(dbruntime.RenameTableSQL(dialect, pair.From, pair.To) + "\n")
		for _, index := range pair.IndexRenames {
			b.WriteString(dbruntime.RenameIndexSQL(dialect, index.Table, index.From, index.To) + "\n")
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
func formatIndexRenames(mode schema.GeneratorMode, pairs []indexRenamePair) string {
	if len(pairs) == 0 {
		return ""
	}
	dialect := dialectFor(mode)

	var b strings.Builder
	b.WriteString("  -- The plan above drops and re-creates the indexes below with identical definitions; these are renames.\n")
	b.WriteString("  -- Renaming an index only modifies metadata; DROP + ADD rebuilds it (full table scan on large tables).\n")
	b.WriteString("  -- Run the statement(s) below instead, then re-run gg migrate.\n")
	for _, pair := range pairs {
		unique := ""
		if pair.Unique {
			unique = ", UNIQUE"
		}
		fmt.Fprintf(&b, "  -- Table %s: %s -> %s (%s%s)\n", dbruntime.QuoteIdentifier(dialect, pair.Table), dbruntime.QuoteIdentifier(dialect, pair.From), dbruntime.QuoteIdentifier(dialect, pair.To), pair.Columns, unique)
	}
	b.WriteString("\n")
	for _, pair := range pairs {
		b.WriteString(dbruntime.RenameIndexSQL(dialect, pair.Table, pair.From, pair.To) + "\n")
	}
	return b.String()
}
