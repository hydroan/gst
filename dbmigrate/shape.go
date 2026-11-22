package dbmigrate

import (
	"strings"

	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/parser"
)

// schemaShape is a schema reduced to what a migration compares by name: its
// tables, each with its columns and its secondary indexes. It is read off
// the statements sqldef's parser parses, so the models' schema and the
// schema a database exports are read the same way, whichever spelling each
// arrives in.
type schemaShape struct {
	// tables holds the tables by their bare name in lower case, the schema
	// qualifier a PostgreSQL export writes left out.
	tables map[string]*tableShape
	// names lists the tables by name as their statements spell it, in the
	// order the statements come.
	names []string
}

// tableShape is one table of a schemaShape.
type tableShape struct {
	name    string
	columns []columnShape
	indexes []indexShape
	// checks are the table-level CHECK constraints, each as the parser
	// renders it, on one line and in lower case.
	checks []string
}

// columnShape is one column: its name, the definition the parser renders for
// it, its type, nullability, default and the rest, and whether the table's
// PRIMARY KEY clause names it. A primary key column is not null without
// saying so, which its definition alone does not tell: the models' schema
// writes `id` varchar(36) under PRIMARY KEY (`id`), and the database exports
// the column as varchar(36) NOT NULL.
type columnShape struct {
	name       string
	definition string
	primary    bool
}

// indexShape is one secondary index: its name, its definition spelled to
// compare (see indexDefinition), and whether a statement of its own declares
// it, CREATE INDEX or ALTER TABLE ... ADD INDEX, and not a line of its
// table's CREATE TABLE.
type indexShape struct {
	name       string
	definition string
	standalone bool
}

// readShape reads the tables of ddls with their columns, CHECK constraints
// and secondary indexes. For
//
//	CREATE TABLE `samples` (`id` char(36) NOT NULL, `code` varchar(64), PRIMARY KEY (`id`));
//	CREATE UNIQUE INDEX `uniq_samples_code` ON `samples` (`code`);
//
// it returns the table samples with the columns id, "char(36) not null" and
// of the primary key, and code, "varchar(64)", and the index
// uniq_samples_code, "unique code". The primary key is no secondary index,
// and an index statement naming a table ddls does not create is left out.
func readShape(sqlParser database.Parser, ddls string) (schemaShape, error) {
	statements, err := sqlParser.Parse(ddls)
	if err != nil {
		return schemaShape{}, err
	}
	shape := schemaShape{tables: make(map[string]*tableShape)}
	for _, statement := range statements {
		ddl, ok := statement.Statement.(*parser.DDL)
		if !ok {
			continue
		}
		switch ddl.Action {
		case parser.CreateTable:
			if ddl.TableSpec == nil {
				continue
			}
			table := &tableShape{name: ddl.NewName.Name.Name}
			for _, column := range ddl.TableSpec.Columns {
				table.columns = append(table.columns, columnShape{name: column.Name.Name, definition: parser.String(&column.Type)})
			}
			for _, index := range ddl.TableSpec.Indexes {
				if index.Info == nil {
					continue
				}
				if index.Info.Primary {
					table.primaryKey(index.Columns)
					continue
				}
				table.indexes = append(table.indexes, indexShape{
					name:       index.Info.Name.Name,
					definition: indexDefinition(index.Info.Unique, index.Columns, nil),
				})
			}
			for _, check := range ddl.TableSpec.Checks {
				table.checks = append(table.checks, normalizeDefinition(parser.String(check)))
			}
			shape.tables[strings.ToLower(table.name)] = table
			shape.names = append(shape.names, table.name)
		case parser.AddPrimaryKey:
			if table := shape.table(ddl.Table.Name.Name); table != nil {
				table.primaryKey(ddl.IndexCols)
			}
		case parser.CreateIndex, parser.AddIndex:
			table := shape.table(ddl.Table.Name.Name)
			if table == nil || ddl.IndexSpec == nil || ddl.IndexSpec.Primary {
				continue
			}
			table.indexes = append(table.indexes, indexShape{
				name:       ddl.IndexSpec.Name.Name,
				definition: indexDefinition(ddl.IndexSpec.Unique, ddl.IndexCols, ddl.IndexSpec.Where),
				standalone: true,
			})
		}
	}
	return shape, nil
}

// table returns the table named name, whatever the case of the name, or nil
// when the schema has none.
func (s schemaShape) table(name string) *tableShape {
	return s.tables[strings.ToLower(name)]
}

// primaryKey marks the columns a PRIMARY KEY clause names.
func (t *tableShape) primaryKey(columns []parser.IndexColumn) {
	for _, key := range columns {
		for i := range t.columns {
			if strings.EqualFold(t.columns[i].name, key.Column.Name) {
				t.columns[i].primary = true
			}
		}
	}
}

// column returns the column named name, whatever the case of the name.
func (t *tableShape) column(name string) (columnShape, bool) {
	for _, column := range t.columns {
		if strings.EqualFold(column.name, name) {
			return column, true
		}
	}
	return columnShape{}, false
}

// index returns the index named name, whatever the case of the name.
func (t *tableShape) index(name string) (indexShape, bool) {
	for _, index := range t.indexes {
		if strings.EqualFold(index.name, name) {
			return index, true
		}
	}
	return indexShape{}, false
}

// indexDefinition spells the definition of an index so that two spellings of
// one definition are the same text: whether it is unique, its key parts in
// order, a column by its name in lower case with its prefix length and a
// descending order, an expression as normalizeCondition leaves it, and the
// condition of a partial index the same way. A unique index over the column
// code of the rows not soft-deleted reads
// "unique code where deleted_at is null" on PostgreSQL and SQLite, and
// "unique code,case when deleted_at is null then 1 end" on MySQL, in the
// models' schema and in the schema the database exports alike.
func indexDefinition(unique bool, columns []parser.IndexColumn, where *parser.Where) string {
	var b strings.Builder
	if unique {
		b.WriteString("unique ")
	}
	for i, column := range columns {
		if i > 0 {
			b.WriteByte(',')
		}
		if column.Expression != nil {
			b.WriteString(normalizeCondition(parser.String(column.Expression)))
		} else {
			b.WriteString(strings.ToLower(column.Column.Name))
		}
		if column.Length != nil {
			b.WriteString("(" + parser.String(column.Length) + ")")
		}
		if strings.EqualFold(column.Direction, "desc") {
			b.WriteString(" desc")
		}
	}
	if where != nil && where.Expr != nil {
		b.WriteString(" where " + normalizeCondition(parser.String(where.Expr)))
	}
	return b.String()
}

// normalizeCondition strips identifier quoting, parentheses, letter case and
// spacing from a condition or an expression, so its spellings compare by
// content: PostgreSQL exports (deleted_at IS NULL) for the condition the
// models' schema spells "deleted_at" IS NULL, and both normalize to
// deleted_at is null.
func normalizeCondition(condition string) string {
	condition = strings.NewReplacer("`", "", `"`, "", "(", " ", ")", " ").Replace(condition)
	return strings.ToLower(strings.Join(strings.Fields(condition), " "))
}

// normalizeDefinition folds a rendered definition onto one line, in lower
// case, so two renderings of one definition compare equal.
func normalizeDefinition(definition string) string {
	return strings.ToLower(strings.Join(strings.Fields(definition), " "))
}
