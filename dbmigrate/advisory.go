package dbmigrate

import (
	"fmt"
	"strings"
)

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
// SQL comment prefix so pasting the whole block into MySQL stays harmless,
// and only directly executable statements appear unprefixed, grouped at the
// end after a blank line. Each RENAME TABLE precedes the RENAME INDEX
// statements of its own table, which only apply once the table is renamed.
// Remaining changes render as comments only: they belong to the next
// migration run, not to the rename.
func formatTableRenames(pairs []tableRenamePair) string {
	if len(pairs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("  -- The plan above drops and re-creates the tables below; the created table keeps every column of the dropped one, so these are renames.\n")
	b.WriteString("  -- DROP TABLE discards every row; RENAME TABLE only modifies metadata and keeps the data.\n")
	b.WriteString("  -- Run the statement(s) below instead, then re-run gg migrate.\n")
	for _, pair := range pairs {
		if len(pair.Residual) > 0 {
			b.WriteString("  -- Changes marked \"remaining\" are not covered by the rename; re-run gg migrate after renaming to apply them as in-place ALTERs.\n")
			break
		}
	}
	for _, pair := range pairs {
		fmt.Fprintf(&b, "  -- Table `%s` -> `%s`\n", pair.From, pair.To)
		for _, index := range pair.IndexRenames {
			unique := ""
			if index.Unique {
				unique = ", UNIQUE"
			}
			fmt.Fprintf(&b, "  --   index `%s` -> `%s` (%s%s)\n", index.From, index.To, index.Columns, unique)
		}
		for _, statement := range pair.Residual {
			fmt.Fprintf(&b, "  --   remaining change: %s\n", statement)
		}
	}
	b.WriteString("\n")
	for _, pair := range pairs {
		fmt.Fprintf(&b, "RENAME TABLE `%s` TO `%s`;\n", pair.From, pair.To)
		for _, index := range pair.IndexRenames {
			fmt.Fprintf(&b, "ALTER TABLE `%s` RENAME INDEX `%s` TO `%s`;\n", index.Table, index.From, index.To)
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
// SQL comment prefix so pasting the whole block into MySQL stays harmless,
// and only directly executable RENAME statements appear unprefixed, grouped
// at the end after a blank line.
func formatIndexRenames(pairs []indexRenamePair) string {
	if len(pairs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("  -- The plan above drops and re-creates the indexes below with identical definitions; these are renames.\n")
	b.WriteString("  -- RENAME INDEX only modifies metadata; DROP + ADD rebuilds the index (full table scan on large tables).\n")
	b.WriteString("  -- Run the statement(s) below instead, then re-run gg migrate.\n")
	for _, pair := range pairs {
		unique := ""
		if pair.Unique {
			unique = ", UNIQUE"
		}
		fmt.Fprintf(&b, "  -- Table `%s`: `%s` -> `%s` (%s%s)\n", pair.Table, pair.From, pair.To, pair.Columns, unique)
	}
	b.WriteString("\n")
	for _, pair := range pairs {
		fmt.Fprintf(&b, "ALTER TABLE `%s` RENAME INDEX `%s` TO `%s`;\n", pair.Table, pair.From, pair.To)
	}
	return b.String()
}
