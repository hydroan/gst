package dbmigrate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/schema"
)

// tableRenamePair is one confirmed table rename: renaming the dropped table
// to the created name loses no data, because the created table carries every
// column of the dropped one (verified by re-diffing the pair through sqldef
// itself), so a metadata-only RENAME TABLE can replace the drop-and-recreate
// pair. IndexRenames carries the index renames that follow the table rename,
// because generated index names embed the table name and necessarily change
// with it. Residual carries the allowed leftover statements — column
// additions and unpaired index changes — that the next migration run applies
// as in-place ALTERs once the table is renamed.
type tableRenamePair struct {
	From         string
	To           string
	IndexRenames []renamePair
	Residual     []string
}

// dropTablePattern matches the DROP TABLE statement shape emitted by the
// sqldef MySQL generator. DROP statements never carry the table definition;
// it is recovered from the exported current schema instead.
var dropTablePattern = regexp.MustCompile("^DROP TABLE ([^ ]+)$")

// addColumnPattern matches the one non-index statement shape allowed to
// remain after a rename: adding a column keeps every current row, so the
// created table stays a data-preserving superset of the dropped one.
var addColumnPattern = regexp.MustCompile("^ALTER TABLE [^ ]+ ADD COLUMN .+$")

// detectTableRenames pairs every table the migration plan would create with a
// dropped table whose data the rename would fully preserve, recovering the
// dropped side's definition from the exported current schema. A candidate
// pair is verified by rewriting the dropped table's definition to the created
// table's name and re-diffing the two through sqldef itself, so the
// comparison normalizes exactly like the migration plan did. The pair is a
// rename when the residual diff cannot lose data: verified index renames
// follow the table rename, other index changes and column additions are
// reported as remaining changes, and anything else — dropped or modified
// columns, table options — fails closed as a genuine rebuild. Only pairs with
// an unambiguous one-to-one pairing are reported, so every reported pair is
// safe to rename.
func detectTableRenames(generatorMode schema.GeneratorMode, sqlParser database.Parser, config database.GeneratorConfig, defaultSchema string, ddls []string, currentDDLs string) []tableRenamePair {
	dropped := make([]string, 0)
	added := make([]string, 0)
	addedTables := make(map[string]string)
	addedIndexes := make(map[string][]string)
	for _, ddl := range ddls {
		statement := strings.TrimSpace(ddl)
		if m := dropTablePattern.FindStringSubmatch(statement); m != nil {
			dropped = append(dropped, unquoteIdent(m[1]))
			continue
		}
		if m := currentTablePattern.FindStringSubmatch(statement); m != nil {
			added = append(added, m[1])
			addedTables[m[1]] = statement
			continue
		}
		if m := createIndexPattern.FindStringSubmatch(statement); m != nil {
			table := unquoteIdent(m[3])
			addedIndexes[table] = append(addedIndexes[table], statement)
		}
	}
	if len(dropped) == 0 || len(added) == 0 {
		return nil
	}

	currentTables := parseCurrentTables(currentDDLs)
	candidates := make([]tableRenamePair, 0)
	for _, to := range added {
		desired := strings.Join(append([]string{addedTables[to]}, addedIndexes[to]...), ";\n")
		for _, from := range dropped {
			if from == to {
				continue
			}
			current, ok := currentTables[from]
			if !ok {
				continue
			}
			indexRenames, residual, ok := classifyTableRename(generatorMode, sqlParser, config, defaultSchema, desired, renameCurrentTable(current, to))
			if !ok {
				continue
			}
			candidates = append(candidates, tableRenamePair{From: from, To: to, IndexRenames: indexRenames, Residual: residual})
		}
	}

	// A dropped or created table claimed by more than one candidate is
	// ambiguous; such tables report no pairing at all.
	fromCount := make(map[string]int)
	toCount := make(map[string]int)
	for _, candidate := range candidates {
		fromCount[candidate.From]++
		toCount[candidate.To]++
	}
	pairs := make([]tableRenamePair, 0)
	for _, candidate := range candidates {
		if fromCount[candidate.From] != 1 || toCount[candidate.To] != 1 {
			continue
		}
		pairs = append(pairs, candidate)
	}
	return pairs
}

// classifyTableRename re-diffs one candidate pair through sqldef: desired is
// the created table's plan statements and current is the dropped table's
// exported definition rewritten to the created name. The pair is a rename
// when every residual statement is proven data-preserving: index changes
// never touch row data, and ADD COLUMN only extends it. Verified index
// drop/add pairs become the RENAME INDEX guidance; the other allowed
// statements are returned as remaining changes. Anything else fails closed as
// a genuine rebuild, as does a sqldef error, because the advisory must never
// block the migration plan. EnableDrop is forced on so the residual keeps its
// destructive statements; without them a data-losing mismatch could pass as a
// rename.
func classifyTableRename(generatorMode schema.GeneratorMode, sqlParser database.Parser, config database.GeneratorConfig, defaultSchema string, desiredDDLs string, currentDDL string) (indexRenames []renamePair, residual []string, ok bool) {
	if len(currentDDL) == 0 {
		return nil, nil, false
	}
	config.EnableDrop = true
	statements, err := schema.GenerateIdempotentDDLs(generatorMode, sqlParser, desiredDDLs, currentDDL+";", config, defaultSchema)
	if err != nil {
		return nil, nil, false
	}

	indexStatements := make([]string, 0, len(statements))
	for _, statement := range statements {
		trimmed := strings.TrimSpace(statement)
		switch {
		case isIndexStatement(trimmed):
			indexStatements = append(indexStatements, trimmed)
		case addColumnPattern.MatchString(trimmed):
		default:
			return nil, nil, false
		}
	}

	indexRenames = detectIndexRenames(indexStatements, currentDDL)
	if len(indexRenames) == 0 {
		indexRenames = nil
	}
	for _, statement := range statements {
		trimmed := strings.TrimSpace(statement)
		if isIndexStatement(trimmed) && isConsumedByIndexRename(trimmed, indexRenames) {
			continue
		}
		residual = append(residual, trimmed)
	}
	return indexRenames, residual, true
}

// isIndexStatement reports whether the statement drops or adds a secondary
// index, in any of the shapes the sqldef MySQL generator emits.
func isIndexStatement(statement string) bool {
	return dropIndexPattern.MatchString(statement) ||
		alterAddPattern.MatchString(statement) ||
		createIndexPattern.MatchString(statement)
}

// isConsumedByIndexRename reports whether the statement is one side of a
// verified index rename pair, and therefore already covered by the pair's
// RENAME INDEX guidance.
func isConsumedByIndexRename(statement string, renames []renamePair) bool {
	if m := dropIndexPattern.FindStringSubmatch(statement); m != nil {
		table, name := unquoteIdent(m[1]), unquoteIdent(m[2])
		for _, rename := range renames {
			if rename.Table == table && rename.From == name {
				return true
			}
		}
		return false
	}
	table, name := "", ""
	if m := alterAddPattern.FindStringSubmatch(statement); m != nil {
		table, name = unquoteIdent(m[1]), unquoteIdent(m[3])
	} else if m := createIndexPattern.FindStringSubmatch(statement); m != nil {
		table, name = unquoteIdent(m[3]), unquoteIdent(m[2])
	}
	for _, rename := range renames {
		if rename.Table == table && rename.To == name {
			return true
		}
	}
	return false
}

// renameCurrentTable rewrites only the table name inside the CREATE TABLE
// header, preserving the quoting around it, so the dropped table's exported
// definition can be compared against the created table.
func renameCurrentTable(statement, to string) string {
	loc := currentTablePattern.FindStringSubmatchIndex(statement)
	if loc == nil {
		return ""
	}
	return statement[:loc[2]] + to + statement[loc[3]:]
}

// parseCurrentTables extracts each table's full CREATE TABLE statement from
// the SHOW CREATE TABLE style DDLs exported from the current database. The
// trailing statement terminator is stripped so the definition can be embedded
// in new statement lists.
func parseCurrentTables(currentDDLs string) map[string]string {
	tables := make(map[string]string)
	table := ""
	lines := make([]string, 0)
	for line := range strings.SplitSeq(currentDDLs, "\n") {
		if m := currentTablePattern.FindStringSubmatch(line); m != nil {
			table = m[1]
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
