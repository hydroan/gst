package dbmigrate

import (
	"regexp"
	"strings"
)

// Statement shapes emitted by the sqldef MySQL and PostgreSQL generators for
// secondary indexes. ADD statements keep the desired-side keyword (INDEX in
// the gorm-generated schema; KEY tolerated for safety) and carry the column
// list, DROP statements never carry column information — their definitions
// are recovered from the exported current schema instead. Covers plain,
// composite, and unique indexes in the ALTER TABLE ADD form (MySQL struct
// tag indexes embedded in CREATE TABLE), the standalone CREATE INDEX form
// (indexes declared through a model's Indexes method; the optional USING
// clause appears in the PostgreSQL export, and the optional condition on a
// unique index over the live rows alone, see indexDefinitionOf), and the bare
// DROP INDEX form the PostgreSQL generator emits, which names no table — the
// owning table is recovered from the exported current schema.
var (
	dropIndexPattern     = regexp.MustCompile(`^ALTER TABLE ([^ ]+) DROP INDEX ([^ ]+)$`)
	bareDropIndexPattern = regexp.MustCompile(`^DROP INDEX ([^ ]+)$`)
	alterAddPattern      = regexp.MustCompile(`^ALTER TABLE ([^ ]+) ADD (UNIQUE )?(?:FULLTEXT |SPATIAL )?(?:INDEX|KEY) ([^ ]+) \((.+)\)$`)
	createIndexPattern   = regexp.MustCompile(`^CREATE (UNIQUE )?INDEX ([^ ]+) ON ([^ ]+)(?: USING [^ ]+)? \((.+?)\)(?: WHERE (.+))?$`)
)

// Statement shapes emitted by the sqldef generators for whole tables.
var (
	// dropTablePattern matches the DROP TABLE statement shape. DROP statements
	// never carry the table definition; it is recovered from the exported
	// current schema instead.
	dropTablePattern = regexp.MustCompile(`^DROP TABLE ([^ ]+)$`)
	// addColumnPattern matches the one non-index statement shape allowed to
	// remain after a table rename: adding a column keeps every current row, so
	// the created table stays a data-preserving superset of the dropped one.
	addColumnPattern = regexp.MustCompile(`^ALTER TABLE [^ ]+ ADD COLUMN .+$`)
)

// Statement shapes of a CREATE TABLE body. They match both the SHOW CREATE
// TABLE style schema exported from the current database and the CREATE TABLE
// statements a migration plan would apply.
var (
	// currentTablePattern matches the CREATE TABLE header of one statement.
	// The quote around the table name — backticks in MySQL and SQLite,
	// double quotes in PostgreSQL — is captured apart from the name, which
	// is the second group, so a rewrite of the name keeps the quoting.
	currentTablePattern = regexp.MustCompile("^\\s*CREATE TABLE ([`\"]?)([^`\" (]+)([`\"]?) \\(")
	// currentIndexPattern matches one secondary index line inside a CREATE
	// TABLE body. The column list is captured up to the line's last closing
	// parenthesis by the caller, so prefix lengths like col(10) survive.
	currentIndexPattern = regexp.MustCompile("^\\s*(UNIQUE |FULLTEXT |SPATIAL )?KEY `([^`]+)` \\((.+)\\)")
)

// indexDefinition is the definition of one secondary index, read off the
// exported current schema or off a statement of the plan (see
// indexDefinitionOf).
type indexDefinition struct {
	Columns  string // normalized column list
	Unique   bool
	LiveOnly bool // whether a unique index holds the live rows alone, those not soft-deleted
}

// The spellings a unique index over the live rows alone is recognized by,
// the ones modelschema.IndexPlan.CreateSQL writes, each as normalizeCondition
// leaves it: the condition PostgreSQL and SQLite index a part of the table
// by, and the key part that tells a live row on MySQL. The plan carries them
// as sqldef prints them back and the exported schema as the database stores
// them, in parentheses and, on MySQL, in lower case; normalized, the
// spellings of one are the same text.
const (
	liveOnlyCondition = "deleted_at is null"
	liveOnlyKeyPart   = "case when deleted_at is null then 1 end"
)

// indexDefinitionOf reads the definition of an index off the column list and
// the condition of its statement, "" for a statement carrying none, as either
// the plan or the exported schema spells them. A unique index over the live
// rows alone carries the live-rows condition on PostgreSQL and SQLite, and on
// MySQL one more key part after its columns, which is none of them: the
// column list "`code`,((case when (`deleted_at` is null) then 1 end))" reads
// as the column code of an index over the live rows. The second result is
// false for a statement under any other condition, an index the framework
// did not build, which no rename guidance is derived from.
func indexDefinitionOf(columns, condition string, unique bool) (indexDefinition, bool) {
	liveOnly := false
	if condition != "" {
		if normalizeCondition(condition) != liveOnlyCondition {
			return indexDefinition{}, false
		}
		liveOnly = true
	} else if i := strings.LastIndex(columns, ","); i >= 0 && normalizeCondition(columns[i+1:]) == liveOnlyKeyPart {
		columns, liveOnly = columns[:i], true
	}
	return indexDefinition{Columns: normalizeColumns(columns), Unique: unique, LiveOnly: liveOnly}, true
}

// flattenStatement collapses a statement onto one line, folding every run of
// whitespace into a single space. The schema dumper pretty-prints statements
// past the formatter's width — a long CREATE INDEX arrives with its column
// list spread across indented lines — and the statement patterns above match
// single lines only, so every plan-side statement flattens before matching.
// The exported current schema never flattens: it is parsed line by line.
func flattenStatement(statement string) string {
	return strings.Join(strings.Fields(statement), " ")
}

// isIndexStatement reports whether the statement drops or adds a secondary
// index, in any of the shapes the sqldef generators emit.
func isIndexStatement(statement string) bool {
	statement = flattenStatement(statement)
	return dropIndexPattern.MatchString(statement) ||
		bareDropIndexPattern.MatchString(statement) ||
		alterAddPattern.MatchString(statement) ||
		createIndexPattern.MatchString(statement)
}

// parseCurrentIndexes extracts the secondary index definitions per table
// from the DDLs exported from the current database: MySQL embeds them as KEY
// lines inside the SHOW CREATE TABLE body, PostgreSQL exports standalone
// CREATE INDEX statements.
func parseCurrentIndexes(currentDDLs string) map[string]map[string]indexDefinition {
	indexes := make(map[string]map[string]indexDefinition)
	ensure := func(table string) map[string]indexDefinition {
		if m, ok := indexes[table]; ok {
			return m
		}
		m := make(map[string]indexDefinition)
		indexes[table] = m
		return m
	}
	table := ""
	for line := range strings.SplitSeq(currentDDLs, "\n") {
		if m := currentTablePattern.FindStringSubmatch(line); m != nil {
			table = unquoteIdent(m[2])
			ensure(table)
			continue
		}
		standalone := strings.TrimSuffix(strings.TrimSpace(line), ";")
		if m := createIndexPattern.FindStringSubmatch(standalone); m != nil {
			if definition, ok := indexDefinitionOf(m[4], m[5], m[1] != ""); ok {
				ensure(unquoteIdent(m[3]))[unquoteIdent(m[2])] = definition
			}
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
		ensure(table)[m[2]], _ = indexDefinitionOf(trimmed[start+1:end], "", strings.TrimSpace(m[1]) == "UNIQUE")
	}
	return indexes
}

// parseCurrentTables extracts each table's full CREATE TABLE statement from
// the DDLs exported from the current database. The trailing statement
// terminator is stripped so the definition can be embedded in new statement
// lists.
func parseCurrentTables(currentDDLs string) map[string]string {
	tables := make(map[string]string)
	table := ""
	lines := make([]string, 0)
	for line := range strings.SplitSeq(currentDDLs, "\n") {
		if m := currentTablePattern.FindStringSubmatch(line); m != nil {
			table = unquoteIdent(m[2])
			lines = append(lines[:0], line)
			continue
		}
		if table == "" {
			continue
		}
		lines = append(lines, line)
		// SHOW CREATE TABLE closes the body on a line of its own; that line
		// carries the table options and the terminator.
		if strings.HasPrefix(strings.TrimSpace(line), ")") {
			statement := strings.TrimSpace(strings.Join(lines, "\n"))
			tables[table] = strings.TrimSuffix(statement, ";")
			table = ""
			lines = lines[:0]
		}
	}
	return tables
}

// normalizeCondition strips identifier quoting, parentheses, letter case and
// spacing from a condition or an expression, so its spellings compare by
// content: PostgreSQL exports (deleted_at IS NULL) for the condition the plan
// spells "deleted_at" IS NULL, and both normalize to deleted_at is null.
func normalizeCondition(condition string) string {
	condition = strings.NewReplacer("`", "", `"`, "", "(", " ", ")", " ").Replace(condition)
	return strings.ToLower(strings.Join(strings.Fields(condition), " "))
}

// normalizeColumns strips identifier quoting and spacing from a column list
// so plan-side and current-side definitions compare by content.
func normalizeColumns(columns string) string {
	columns = strings.ReplaceAll(columns, "`", "")
	columns = strings.ReplaceAll(columns, `"`, "")
	columns = strings.ReplaceAll(columns, " ", "")
	return columns
}

// unquoteIdent strips a leading schema qualifier and identifier quoting
// (MySQL backticks, standard double quotes) from an identifier. A quoted
// identifier that itself contains a dot loses its prefix and then matches
// nothing downstream, which fails safe: an unmatched name never produces
// rename guidance.
func unquoteIdent(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[i+1:]
	}
	return strings.Trim(s, "`\"")
}
