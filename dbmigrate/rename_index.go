package dbmigrate

// indexRenamePair is one confirmed index rename: the dropped and the added
// index carry exactly the same column sequence and uniqueness, so a
// metadata-only RENAME INDEX can replace the drop-and-rebuild pair.
type indexRenamePair struct {
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

// detectIndexRenames pairs every added index in the migration plan with a
// dropped index of the identical definition, recovering the dropped side's
// columns from the exported current schema. Only exact matches (same table,
// same column sequence, same uniqueness) with an unambiguous one-to-one
// pairing are reported, so every reported pair is safe to rename.
//
// The PostgreSQL generator drops indexes through a bare DROP INDEX that
// names no table; the owning table is recovered from the exported current
// schema, where index names are unique per schema.
func detectIndexRenames(ddls []string, currentDDLs string) []indexRenamePair {
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

	current := parseCurrentIndexes(currentDDLs)
	tableByIndex := make(map[string]string)
	for table, defs := range current {
		for name := range defs {
			if _, dup := tableByIndex[name]; dup {
				// The same index name on two tables (possible on MySQL) is
				// ambiguous for a bare DROP INDEX; report no owner at all.
				tableByIndex[name] = ""
				continue
			}
			tableByIndex[name] = table
		}
	}

	for _, ddl := range ddls {
		statement := flattenStatement(ddl)
		if m := dropIndexPattern.FindStringSubmatch(statement); m != nil {
			c := track(unquoteIdent(m[1]))
			c.dropped = append(c.dropped, unquoteIdent(m[2]))
			continue
		}
		if m := bareDropIndexPattern.FindStringSubmatch(statement); m != nil {
			name := unquoteIdent(m[1])
			if table := tableByIndex[name]; table != "" {
				c := track(table)
				c.dropped = append(c.dropped, name)
			}
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

	pairs := make([]indexRenamePair, 0)
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
			pairs = append(pairs, indexRenamePair{
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
