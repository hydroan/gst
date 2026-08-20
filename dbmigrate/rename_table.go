package dbmigrate

import (
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
	IndexRenames []indexRenamePair
	Residual     []string
}

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
func classifyTableRename(generatorMode schema.GeneratorMode, sqlParser database.Parser, config database.GeneratorConfig, defaultSchema string, desiredDDLs string, currentDDL string) (indexRenames []indexRenamePair, residual []string, ok bool) {
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

// isConsumedByIndexRename reports whether the statement is one side of a
// verified index rename pair, and therefore already covered by the pair's
// RENAME INDEX guidance.
func isConsumedByIndexRename(statement string, renames []indexRenamePair) bool {
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
