package dbmigrate

import (
	"fmt"
	"regexp"
	"strings"
)

// indexRenameHint groups the dropped and added secondary indexes that one
// migration plan touches on a single table. When both sides are non-empty,
// some drop/add pairs may actually be renames that should run as RENAME
// INDEX instead of a full drop-and-rebuild.
type indexRenameHint struct {
	Table   string
	Dropped []string
	Added   []addedIndex
}

// addedIndex is one secondary index the migration plan would create.
type addedIndex struct {
	Name    string
	Columns string // raw column list as it appears in the plan, e.g. "`kind`, `created_at`"
	Unique  bool
}

// Statement shapes emitted by the sqldef MySQL generator for secondary
// indexes. ADD statements keep the desired-side keyword (INDEX in the
// gorm-generated schema; KEY tolerated for safety) and carry the column
// list, DROP statements never carry column information. Covers plain,
// composite, and unique indexes in both the ALTER TABLE ADD form (struct
// tag indexes embedded in CREATE TABLE) and the standalone CREATE INDEX
// form (Indexer capability indexes).
var (
	dropIndexPattern   = regexp.MustCompile("^ALTER TABLE ([^ ]+) DROP INDEX ([^ ]+)$")
	alterAddPattern    = regexp.MustCompile(`^ALTER TABLE ([^ ]+) ADD (UNIQUE )?(?:FULLTEXT |SPATIAL )?(?:INDEX|KEY) ([^ ]+) \((.+)\)$`)
	createIndexPattern = regexp.MustCompile(`^CREATE (UNIQUE )?INDEX ([^ ]+) ON ([^ ]+) \((.+)\)$`)
)

// detectIndexRenameHints scans a migration plan for tables that both drop
// and add secondary indexes. It only reads the plan text: DROP statements
// carry no column information, so pairing drops with adds stays a human
// decision and the hint reports both sides for review.
func detectIndexRenameHints(ddls []string) []indexRenameHint {
	type tableChanges struct {
		dropped []string
		added   []addedIndex
	}
	changes := make(map[string]*tableChanges)
	tables := make([]string, 0)
	track := func(table string) *tableChanges {
		if c, ok := changes[table]; ok {
			return c
		}
		c := &tableChanges{}
		changes[table] = c
		tables = append(tables, table)
		return c
	}

	for _, ddl := range ddls {
		statement := strings.TrimSpace(ddl)
		if m := dropIndexPattern.FindStringSubmatch(statement); m != nil {
			c := track(unquoteIdent(m[1]))
			c.dropped = append(c.dropped, unquoteIdent(m[2]))
			continue
		}
		if m := alterAddPattern.FindStringSubmatch(statement); m != nil {
			c := track(unquoteIdent(m[1]))
			c.added = append(c.added, addedIndex{Name: unquoteIdent(m[3]), Columns: m[4], Unique: m[2] != ""})
			continue
		}
		if m := createIndexPattern.FindStringSubmatch(statement); m != nil {
			c := track(unquoteIdent(m[3]))
			c.added = append(c.added, addedIndex{Name: unquoteIdent(m[2]), Columns: m[4], Unique: m[1] != ""})
		}
	}

	hints := make([]indexRenameHint, 0, len(tables))
	for _, table := range tables {
		c := changes[table]
		if len(c.dropped) != 0 && len(c.added) != 0 {
			hints = append(hints, indexRenameHint{Table: table, Dropped: c.dropped, Added: c.added})
		}
	}
	return hints
}

// formatIndexRenameHints renders the review guidance printed alongside a
// migration plan that drops and re-creates indexes on the same table. It
// only guides; executing RENAME INDEX stays a human decision.
func formatIndexRenameHints(hints []indexRenameHint) string {
	if len(hints) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("-- Possible index rename(s) detected --\n")
	for _, hint := range hints {
		if len(hint.Dropped) == 1 && len(hint.Added) == 1 {
			added := hint.Added[0]
			fmt.Fprintf(&b, "Table `%s`: DROP `%s` + ADD %s (%s) may be a rename.\n",
				hint.Table, hint.Dropped[0], describeAddedIndex(added), added.Columns)
			b.WriteString("If it is, verify the column definitions match, run the statement below manually, then re-run gg migrate:\n")
			fmt.Fprintf(&b, "  ALTER TABLE `%s` RENAME INDEX `%s` TO `%s`;\n", hint.Table, hint.Dropped[0], added.Name)
		} else {
			fmt.Fprintf(&b, "Table `%s`: dropped %s; added %s — some pairs may be renames.\n",
				hint.Table, describeDroppedIndexes(hint.Dropped), describeAddedIndexes(hint.Added))
			fmt.Fprintf(&b, "For each real rename, run `ALTER TABLE `%s` RENAME INDEX <old> TO <new>;` manually, then re-run gg migrate.\n",
				hint.Table)
		}
	}
	b.WriteString("RENAME INDEX only modifies metadata; DROP + ADD rebuilds the index with a full table scan on large tables.\n")
	return b.String()
}

func describeAddedIndex(added addedIndex) string {
	if added.Unique {
		return "UNIQUE `" + added.Name + "`"
	}
	return "`" + added.Name + "`"
}

func describeAddedIndexes(added []addedIndex) string {
	parts := make([]string, 0, len(added))
	for _, a := range added {
		parts = append(parts, fmt.Sprintf("%s (%s)", describeAddedIndex(a), a.Columns))
	}
	return strings.Join(parts, ", ")
}

func describeDroppedIndexes(dropped []string) string {
	parts := make([]string, 0, len(dropped))
	for _, name := range dropped {
		parts = append(parts, "`"+name+"`")
	}
	return strings.Join(parts, ", ")
}

// unquoteIdent strips MySQL backtick quoting from an identifier.
func unquoteIdent(s string) string {
	return strings.Trim(s, "`")
}
