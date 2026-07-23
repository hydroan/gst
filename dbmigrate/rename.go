package dbmigrate

import (
	"fmt"
	"regexp"
	"strings"
)

// renamePair is one confirmed index rename: the dropped and the added index
// carry exactly the same column sequence and uniqueness, so a metadata-only
// RENAME INDEX can replace the drop-and-rebuild pair.
type renamePair struct {
	Table   string
	From    string
	To      string
	Columns string // normalized column list for display, e.g. "group_id,created_at"
	Unique  bool
}

// addedIndex is one secondary index the migration plan would create.
type addedIndex struct {
	Name    string
	Columns string // raw column list as it appears in the plan
	Unique  bool
}

// currentIndex is one secondary index parsed from the exported current schema.
type currentIndex struct {
	Columns string // normalized column list
	Unique  bool
}

// Statement shapes emitted by the sqldef MySQL generator for secondary
// indexes. ADD statements keep the desired-side keyword (INDEX in the
// gorm-generated schema; KEY tolerated for safety) and carry the column
// list, DROP statements never carry column information — their definitions
// are recovered from the exported current schema instead. Covers plain,
// composite, and unique indexes in both the ALTER TABLE ADD form (struct
// tag indexes embedded in CREATE TABLE) and the standalone CREATE INDEX
// form (Indexer capability indexes).
var (
	dropIndexPattern   = regexp.MustCompile("^ALTER TABLE ([^ ]+) DROP INDEX ([^ ]+)$")
	alterAddPattern    = regexp.MustCompile(`^ALTER TABLE ([^ ]+) ADD (UNIQUE )?(?:FULLTEXT |SPATIAL )?(?:INDEX|KEY) ([^ ]+) \((.+)\)$`)
	createIndexPattern = regexp.MustCompile(`^CREATE (UNIQUE )?INDEX ([^ ]+) ON ([^ ]+) \((.+)\)$`)

	// currentTablePattern matches the CREATE TABLE header of one SHOW CREATE
	// TABLE style statement in the exported current schema.
	currentTablePattern = regexp.MustCompile("^\\s*CREATE TABLE `?([^` (]+)`? \\(")
	// currentIndexPattern matches one secondary index line inside a CREATE
	// TABLE body. The column list is captured up to the line's last closing
	// parenthesis by the caller, so prefix lengths like col(10) survive.
	currentIndexPattern = regexp.MustCompile("^\\s*(UNIQUE |FULLTEXT |SPATIAL )?KEY `([^`]+)` \\((.+)\\)")
)

// detectIndexRenames pairs every added index in the migration plan with a
// dropped index of the identical definition, recovering the dropped side's
// columns from the exported current schema. Only exact matches (same table,
// same column sequence, same uniqueness) with an unambiguous one-to-one
// pairing are reported, so every reported pair is safe to rename.
func detectIndexRenames(ddls []string, currentDDLs string) []renamePair {
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

	current := parseCurrentIndexes(currentDDLs)
	pairs := make([]renamePair, 0)
	for _, table := range tables {
		c := changes[table]
		if len(c.dropped) == 0 || len(c.added) == 0 {
			continue
		}
		for _, added := range c.added {
			definition := currentIndex{Columns: normalizeColumns(added.Columns), Unique: added.Unique}
			from, ok := matchDroppedIndex(current[table], c.dropped, definition)
			if !ok {
				continue
			}
			pairs = append(pairs, renamePair{
				Table:   table,
				From:    from,
				To:      added.Name,
				Columns: definition.Columns,
				Unique:  added.Unique,
			})
		}
	}
	return pairs
}

// matchDroppedIndex returns the single dropped index whose current
// definition equals the added one. Zero matches means a genuine rebuild and
// multiple matches are ambiguous; both report no pairing.
func matchDroppedIndex(currentTable map[string]currentIndex, dropped []string, definition currentIndex) (string, bool) {
	matched := ""
	for _, name := range dropped {
		def, ok := currentTable[name]
		if !ok || def != definition {
			continue
		}
		if matched != "" {
			return "", false
		}
		matched = name
	}
	return matched, matched != ""
}

// parseCurrentIndexes extracts the secondary index definitions per table
// from the SHOW CREATE TABLE style DDLs exported from the current database.
func parseCurrentIndexes(currentDDLs string) map[string]map[string]currentIndex {
	indexes := make(map[string]map[string]currentIndex)
	table := ""
	for line := range strings.SplitSeq(currentDDLs, "\n") {
		if m := currentTablePattern.FindStringSubmatch(line); m != nil {
			table = m[1]
			indexes[table] = make(map[string]currentIndex)
			continue
		}
		if table == "" {
			continue
		}
		trimmed := strings.TrimRight(strings.TrimSpace(line), ",")
		m := currentIndexPattern.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		// Recapture the column list up to the line's last closing parenthesis
		// so column prefix lengths like col(10) stay intact.
		start := strings.Index(trimmed, "(")
		end := strings.LastIndex(trimmed, ")")
		if start < 0 || end <= start {
			continue
		}
		indexes[table][m[2]] = currentIndex{
			Columns: normalizeColumns(trimmed[start+1 : end]),
			Unique:  strings.TrimSpace(m[1]) == "UNIQUE",
		}
	}
	return indexes
}

// normalizeColumns strips identifier quoting and spacing from a column list
// so plan-side and current-side definitions compare by content.
func normalizeColumns(columns string) string {
	columns = strings.ReplaceAll(columns, "`", "")
	columns = strings.ReplaceAll(columns, " ", "")
	return columns
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
func formatIndexRenames(pairs []renamePair) string {
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

// unquoteIdent strips MySQL backtick quoting from an identifier.
func unquoteIdent(s string) string {
	return strings.Trim(s, "`")
}
