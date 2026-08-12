package dbmigrate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/schema"
)

// tableRenamePair is one confirmed table rename: the dropped and the created
// table carry the same definition (verified by re-diffing the pair through
// sqldef itself), so a metadata-only RENAME TABLE can replace the
// drop-and-recreate pair without discarding the table's rows. IndexRenames
// carries the index renames that follow the table rename, because generated
// index names embed the table name and necessarily change with it.
type tableRenamePair struct {
	From         string
	To           string
	IndexRenames []renamePair
}

// dropTablePattern matches the DROP TABLE statement shape emitted by the
// sqldef MySQL generator. DROP statements never carry the table definition;
// it is recovered from the exported current schema instead.
var dropTablePattern = regexp.MustCompile("^DROP TABLE ([^ ]+)$")

// detectTableRenames pairs every table the migration plan would create with a
// dropped table of the identical definition, recovering the dropped side's
// definition from the exported current schema. A candidate pair is verified
// by rewriting the dropped table's definition to the created table's name and
// re-diffing the two through sqldef itself, so the comparison normalizes
// exactly like the migration plan did: an empty diff is a pure rename, and a
// diff consisting solely of verified index renames still is one. Only exact
// matches with an unambiguous one-to-one pairing are reported, so every
// reported pair is safe to rename.
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
			indexRenames, ok := classifyTableRename(generatorMode, sqlParser, config, defaultSchema, desired, renameCurrentTable(current, to))
			if !ok {
				continue
			}
			candidates = append(candidates, tableRenamePair{From: from, To: to, IndexRenames: indexRenames})
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
// exported definition rewritten to the created name. EnableDrop is forced on
// so the residual diff keeps its DROP INDEX statements; without them a
// definition mismatch could pass as a rename. A sqldef error fails open as
// "not a rename", because the advisory must never block the migration plan.
func classifyTableRename(generatorMode schema.GeneratorMode, sqlParser database.Parser, config database.GeneratorConfig, defaultSchema string, desiredDDLs string, currentDDL string) ([]renamePair, bool) {
	if len(currentDDL) == 0 {
		return nil, false
	}
	config.EnableDrop = true
	residual, err := schema.GenerateIdempotentDDLs(generatorMode, sqlParser, desiredDDLs, currentDDL+";", config, defaultSchema)
	if err != nil {
		return nil, false
	}
	if len(residual) == 0 {
		return nil, true
	}
	indexRenames := detectIndexRenames(residual, currentDDL)
	if len(indexRenames) > 0 && len(residual) == 2*len(indexRenames) {
		return indexRenames, true
	}
	return nil, false
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
func formatTableRenames(pairs []tableRenamePair) string {
	if len(pairs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("  -- The plan above drops and re-creates the tables below with identical definitions; these are renames.\n")
	b.WriteString("  -- DROP TABLE discards every row; RENAME TABLE only modifies metadata and keeps the data.\n")
	b.WriteString("  -- Run the statement(s) below instead, then re-run gg migrate.\n")
	for _, pair := range pairs {
		fmt.Fprintf(&b, "  -- Table `%s` -> `%s`\n", pair.From, pair.To)
		for _, index := range pair.IndexRenames {
			unique := ""
			if index.Unique {
				unique = ", UNIQUE"
			}
			fmt.Fprintf(&b, "  --   index `%s` -> `%s` (%s%s)\n", index.From, index.To, index.Columns, unique)
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
