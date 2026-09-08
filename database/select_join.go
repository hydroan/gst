package database

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/types"
	"gorm.io/gorm"
)

// This file is the join side of the select builder: how a joined model or a
// joined select is resolved and proved to match at most one row, how its ON
// renders, and how the columns of several tables are told apart. See
// types.JoinSource for the contract. The builder itself lives in select.go.

// Errors reported while a join is built; see the select errors for why they
// fail fast.
var (
	ErrJoinNotUnique        = errors.New("join must pin the joined model's primary key or one of its unique indexes, or every group key of the joined select, with equalities, or a row could match several joined rows")
	ErrJoinNoCorrelation    = errors.New("join predicates tie the joined model to no table of the query")
	ErrJoinDuplicateTable   = errors.New("a model can be joined once, and the queried model cannot join itself")
	ErrJoinMeasure          = errors.New("a measure over a joined column must be MIN, MAX or COUNT DISTINCT, the other aggregates would count the joined row once per row of the group")
	ErrJoinSource           = errors.New("join source is not one the framework defines")
	ErrJoinSelectNotGrouped = errors.New("a joined select must be grouped, its group keys are what it is joined on")
	ErrJoinSelectInstance   = errors.New("joined select was opened on another database instance")
	ErrJoinSelectColumn     = errors.New("a joined select is read through its own terms, its model's columns are not columns of the derived table")
	ErrJoinSelectNotKeyed   = errors.New("a grouped select projects a joined select's term only when it groups by the columns the select is joined on")
)

// joinedTable is one source joined to the select, resolved from its source
// and proved when the query is validated: a model, or a grouped select read
// as a derived table.
type joinedTable struct {
	// table is the joined model's table, which the columns of the source are
	// referenced by, and alias the name the source is read under in SQL: the
	// table itself for a model, jN for a derived table.
	table string
	alias string
	left  bool
	on    []types.Filter
	// columns and softDelete describe a joined model; sub and derived a
	// joined select, whose derived table has no soft delete of its own.
	columns    map[string]modelschema.Column
	softDelete string
	sub        nestedSelect
	derived    *derivedInfo
	// info is what the filter renderer knows about the source's columns,
	// and joinColumns the columns of the tables read before it that the ON
	// ties the source to, keyed by table.
	info        tableInfo
	joinColumns map[string]map[string]struct{}
}

// derivedInfo is what a query joining a select reads about it: its model's
// table, whether it groups and by which keys, the aliases it projects, the
// ones that can come back NULL, and its model's columns for classifying the
// keys.
type derivedInfo struct {
	table    string
	grouped  bool
	keys     []types.Term
	columns  map[string]modelschema.Column
	aliases  map[string]struct{}
	nullable map[string]string
}

// resolveJoins turns the join sources into joined tables, in order, and
// proves each one: a model joined once, tied to a table read before it, and
// pinned on a unique key so a row of the query matches at most one joined
// row. The tables read before a join are the queried model's and the joins
// declared ahead of it, which is what lets a chain of joins read through one
// another.
func (a *selector[M, R]) resolveJoins(shape *projectionShape) error {
	if len(a.joins) == 0 {
		return nil
	}
	shape.tables = map[string]tableInfo{shape.main: shape.mainInfo}
	shape.joined = make(map[string]*joinedTable, len(a.joins))
	for i, source := range a.joins {
		var (
			jt   *joinedTable
			keys [][]string
			err  error
		)
		switch s := source.(type) {
		case types.ModelJoin:
			jt, keys, err = a.resolveModelJoin(s)
		case types.SelectJoin:
			jt, keys, err = a.resolveSelectJoin(s, "j"+strconv.Itoa(i))
		default:
			// Unreachable: JoinSource is sealed to the two types above.
			return errors.Wrapf(ErrJoinSource, "%T", source)
		}
		if err != nil {
			return err
		}
		if jt.table == shape.main {
			return errors.Wrapf(ErrJoinDuplicateTable, "%q is the queried table", jt.table)
		}
		if _, dup := shape.joined[jt.table]; dup {
			return errors.Wrapf(ErrJoinDuplicateTable, "%q", jt.table)
		}
		pinned, tied := pinnedColumns(jt, shape.tables)
		if !tied {
			return errors.Wrapf(ErrJoinNoCorrelation, "%q", jt.table)
		}
		if !coversKey(pinned, keys) {
			return errors.Wrapf(ErrJoinNotUnique, "%q is pinned on (%s)", jt.table, strings.Join(sortedColumns(pinned), ", "))
		}
		shape.joins = append(shape.joins, jt)
		shape.joined[jt.table] = jt
		shape.tables[jt.table] = jt.info
	}
	return nil
}

// resolveModelJoin reads a joined model: its table, columns and soft-delete
// column, and the keys it is unique on.
func (a *selector[M, R]) resolveModelJoin(mj types.ModelJoin) (*joinedTable, [][]string, error) {
	// A joined select needs no constraint, its group keys make it unique;
	// a joined model is proved on a constraint, which ClickHouse does not
	// carry.
	if a.db.dialect() == dialectClickHouse {
		return nil, nil, errors.Wrap(ErrUnsupportedOnDialect, "model join: ClickHouse carries no unique constraint a join could be proved on")
	}
	if mj.Model == nil {
		return nil, nil, errors.Wrap(ErrJoinSource, "model join carries no model")
	}
	table := mj.Model.TableName()
	if len(table) == 0 {
		return nil, nil, errors.Wrap(ErrJoinSource, "joined model declares no table name")
	}
	columns, err := modelschema.Columns(reflect.TypeOf(mj.Model))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "resolve columns of joined %T", mj.Model)
	}
	keys, err := uniqueKeysOf(a.db.ins, mj.Model)
	if err != nil {
		return nil, nil, err
	}
	return &joinedTable{
		table:      table,
		alias:      table,
		left:       mj.Left,
		on:         mj.On,
		columns:    columnsByName(columns),
		softDelete: softDeleteColumn(columns),
		info:       tableInfoOf(columns),
	}, keys, nil
}

// resolveSelectJoin reads a joined select: it must be a select of this
// package, opened on the query's own instance, and grouped, and its one key
// is the whole set of its group keys. The derived table is read under alias;
// its columns, for the ON and the query's filters, are the model columns the
// group keys read, each renamed to the alias the key projects under.
func (a *selector[M, R]) resolveSelectJoin(sj types.SelectJoin, alias string) (*joinedTable, [][]string, error) {
	sub, ok := sj.Select.(nestedSelect)
	if !ok {
		return nil, nil, errors.Wrapf(ErrJoinSource, "joined select is a %T, not a select this package built", sj.Select)
	}
	if err := sub.attachError(); err != nil {
		return nil, nil, errors.Wrap(err, "joined select")
	}
	if sub.baseHandle() != a.db.base {
		return nil, nil, ErrJoinSelectInstance
	}
	info, err := sub.describe()
	if err != nil {
		return nil, nil, errors.Wrap(err, "joined select")
	}
	if !info.grouped || len(info.keys) == 0 {
		return nil, nil, ErrJoinSelectNotGrouped
	}
	keyColumns := make([]string, 0, len(info.keys))
	keyInfo := tableInfo{
		columns:     make(map[string]struct{}, len(info.keys)),
		timeColumns: make(map[string]struct{}),
		jsonColumns: make(map[string]struct{}),
		qualify:     alias,
		rename:      make(map[string]string, len(info.keys)),
	}
	for _, key := range info.keys {
		keyColumns = append(keyColumns, key.Column)
		keyInfo.columns[key.Column] = struct{}{}
		keyInfo.rename[key.Column] = termAlias(key)
		if c, known := info.columns[key.Column]; known {
			if modelschema.ClassifyColumn(c.Type) == modelschema.ColumnClassTime {
				keyInfo.timeColumns[key.Column] = struct{}{}
			}
			if modelschema.IsJSONType(c.Type) {
				keyInfo.jsonColumns[key.Column] = struct{}{}
			}
		}
	}
	return &joinedTable{
		table:   info.table,
		alias:   alias,
		left:    sj.Left,
		on:      sj.On,
		sub:     sub,
		derived: &info,
		info:    keyInfo,
	}, [][]string{keyColumns}, nil
}

// pinnedColumns reports the columns of the joined table its ON predicates
// pin with an equality — to a column of a table read before it, or to a
// value — and whether any predicate ties the table to one read before it at
// all. It also records on the joined table which columns of those earlier
// tables the ON ties it to. Only predicates AND-combined at the top level
// pin a column: inside an OR group a column is pinned on one branch only,
// which proves nothing.
func pinnedColumns(jt *joinedTable, before map[string]tableInfo) (map[string]struct{}, bool) {
	pinned := make(map[string]struct{})
	jt.joinColumns = make(map[string]map[string]struct{})
	tied := false
	var walk func(filters []types.Filter)
	walk = func(filters []types.Filter) {
		for _, f := range filters {
			switch f.Op {
			case types.FilterOpAnd:
				if children, ok := f.Value.([]types.Filter); ok {
					walk(children)
				}
			case types.FilterOpEq:
				if ownsColumn(f, jt.table) {
					pinned[f.Column] = struct{}{}
				}
			case types.FilterOpEqCol:
				own, otherTable, otherColumn := eqColSides(f, jt.table)
				if len(own) == 0 {
					continue
				}
				if _, ok := before[otherTable]; !ok {
					continue
				}
				pinned[own] = struct{}{}
				if jt.joinColumns[otherTable] == nil {
					jt.joinColumns[otherTable] = make(map[string]struct{})
				}
				jt.joinColumns[otherTable][otherColumn] = struct{}{}
				tied = true
			}
		}
	}
	walk(jt.on)
	return pinned, tied
}

// ownsColumn reports whether a filter names a column of the table: a filter
// without a table names the table it is applied to.
func ownsColumn(f types.Filter, table string) bool {
	return len(f.Table) == 0 || f.Table == table
}

// eqColSides reads an EqCol predicate from the joined table's side: the
// column it pins on that table, and the table and column on the other side.
// A predicate naming the joined table on neither side, or on both, pins
// nothing.
func eqColSides(f types.Filter, table string) (own, otherTable, otherColumn string) {
	parent, parentTable, ok := eqColParent(f.Value)
	if !ok || len(parent) == 0 {
		return "", "", ""
	}
	switch {
	case ownsColumn(f, table) && parentTable != table:
		return f.Column, parentTable, parent
	case parentTable == table && len(f.Table) > 0 && f.Table != table:
		return parent, f.Table, f.Column
	default:
		return "", "", ""
	}
}

// coversKey reports whether the pinned columns cover one of the keys whole.
func coversKey(pinned map[string]struct{}, keys [][]string) bool {
	for _, key := range keys {
		if len(key) == 0 {
			continue
		}
		covered := true
		for _, column := range key {
			if _, ok := pinned[column]; !ok {
				covered = false
				break
			}
		}
		if covered {
			return true
		}
	}
	return false
}

// sortedColumns lists a column set in a stable order, for an error message.
func sortedColumns(set map[string]struct{}) []string {
	columns := make([]string, 0, len(set))
	for column := range set {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

// uniqueKeyCache memoizes the unique keys per model type; the key space is
// bounded by the model types compiled into the binary, and both sources are
// static declarations, so a cached answer never goes stale.
var uniqueKeyCache sync.Map

// uniqueKeysOf resolves the column sets a row of the model is unique on: its
// primary key, then every unique index, whether declared by the Indexes
// method or a struct tag. The indexes come from the collector the Upsert
// result sync reads, so the two never disagree about what is unique.
func uniqueKeysOf(ins *gorm.DB, model any) ([][]string, error) {
	typ := reflect.TypeOf(model)
	if cached, ok := uniqueKeyCache.Load(typ); ok {
		return cached.([][]string), nil //nolint:errcheck
	}
	stmt := &gorm.Statement{DB: ins}
	if err := stmt.Parse(model); err != nil {
		return nil, errors.Wrapf(err, "parse schema of %T", model)
	}
	plans, err := modelregistry.ParseIndexPlans(ins, model)
	if err != nil {
		return nil, err
	}
	indexes, err := collectSaveResultSyncUniqueIndexes(stmt.Schema, plans)
	if err != nil {
		return nil, err
	}
	keys := make([][]string, 0, len(indexes)+1)
	if len(stmt.Schema.PrimaryFieldDBNames) > 0 {
		keys = append(keys, append([]string(nil), stmt.Schema.PrimaryFieldDBNames...))
	}
	for _, index := range indexes {
		key := make([]string, 0, len(index.Fields))
		for _, field := range index.Fields {
			key = append(key, field.Field.DBName)
		}
		keys = append(keys, key)
	}
	uniqueKeyCache.Store(typ, keys)
	return keys, nil
}

// deletedAtType is the type gorm soft deletes through; a model carrying a
// column of it hides its deleted rows from every read.
var deletedAtType = reflect.TypeFor[gorm.DeletedAt]()

// softDeleteColumn returns the soft-delete column of a model, or "" when it
// has none.
func softDeleteColumn(columns []modelschema.Column) string {
	for _, c := range columns {
		if c.Type == deletedAtType {
			return c.DBName
		}
	}
	return ""
}

// columnsByName indexes a model's columns by database name.
func columnsByName(columns []modelschema.Column) map[string]modelschema.Column {
	byName := make(map[string]modelschema.Column, len(columns))
	for _, c := range columns {
		byName[c.DBName] = c
	}
	return byName
}

// joinClauses adds the JOIN clauses to the statement, each ON rendered by
// the filter renderer in the joined source's own scope. A joined model
// carries its soft-delete condition in the ON, so a join never reads a row a
// List on that model hides; it sits in the ON rather than the WHERE because
// in the WHERE it would turn a LEFT JOIN back into an inner one. A joined
// select is rendered as a derived table, its own statement bound into the
// join.
func (a *selector[M, R]) joinClauses(tx *gorm.DB, shape projectionShape) (*gorm.DB, error) {
	before := map[string]tableInfo{shape.main: shape.mainInfo}
	for _, jt := range shape.joins {
		expr, err := a.db.renderFilters(jt.on, false, a.joinScope(jt, before))
		if err != nil {
			return nil, errors.Wrapf(err, "join %q", jt.table)
		}
		kind := "JOIN"
		if jt.left {
			kind = "LEFT JOIN"
		}
		if jt.sub != nil {
			sub, err := jt.sub.buildBranch(buildBranchRead, nil, 0)
			if err != nil {
				return nil, errors.Wrapf(err, "joined select %q", jt.table)
			}
			tx = tx.Joins(kind+" (?) AS "+jt.alias+" ON ?", sub, expr)
		} else {
			sql := kind + " " + a.db.quoteIdent(jt.table) + " ON ?"
			if len(jt.softDelete) > 0 {
				sql += " AND " + a.db.quoteTableColumn(jt.table, jt.softDelete) + " IS NULL"
			}
			tx = tx.Joins(sql, expr)
		}
		before[jt.table] = jt.info
	}
	return tx, nil
}

// joinScope is the scope the ON predicates of a joined source render in: the
// source is the scope's own table, read under its alias and, for a derived
// table, through the names its keys project under, and the tables read
// before it are the ones a predicate may tie to or name.
func (a *selector[M, R]) joinScope(jt *joinedTable, before map[string]tableInfo) filterScope {
	return filterScope{
		qualify:     jt.alias,
		table:       jt.table,
		parent:      jt.table,
		columns:     jt.info.columns,
		timeColumns: jt.info.timeColumns,
		jsonColumns: jt.info.jsonColumns,
		tables:      before,
		rename:      jt.info.rename,
	}
}

// derivedTerms finds the terms of the projection that a joined select
// projects: the query passes the select's own terms, so a match is the whole
// term, the way isSelected matches. Each is a column of that select's
// derived table.
func (a *selector[M, R]) derivedTerms(shape projectionShape) map[string]*joinedTable {
	derived := make(map[string]*joinedTable)
	for _, jt := range shape.joins {
		if jt.sub == nil {
			continue
		}
		for _, t := range a.terms {
			if jt.sub.selects(t) {
				derived[a.alias(t)] = jt
			}
		}
	}
	return derived
}

// readsDerived reports whether a term is one a joined select projects,
// before the joins are resolved: what ScanOne needs to know to refuse a
// row-level read.
func (a *selector[M, R]) readsDerived(t types.Term) bool {
	for _, source := range a.joins {
		sj, ok := source.(types.SelectJoin)
		if !ok {
			continue
		}
		if sub, ok := sj.Select.(nestedSelect); ok && sub.selects(t) {
			return true
		}
	}
	return false
}

// groupDerivedTerms makes the joined selects' terms group keys of a grouped
// projection. Under GROUP BY every column read must be grouped by or
// aggregated, and a derived column is grouped by: that is exact only when
// the projection groups by the columns the select is joined on, so that the
// derived row, hence the term, is constant within a group, which is checked
// before the keys are added.
func (a *selector[M, R]) groupDerivedTerms(shape *projectionShape) error {
	if !shape.grouped || len(shape.derived) == 0 {
		return nil
	}
	for _, t := range a.terms {
		jt, derived := shape.derived[a.alias(t)]
		if !derived {
			continue
		}
		for table, columns := range jt.joinColumns {
			for column := range columns {
				if !a.groupsBy(table, column, *shape) {
					return errors.Wrapf(ErrJoinSelectNotKeyed, "%q reads %q, which is joined on %q of %q", a.alias(t), jt.table, column, table)
				}
			}
		}
		shape.keys = append(shape.keys, t)
	}
	return nil
}

// groupsBy reports whether a column of a table read by the query is one of
// the projection's group keys: a plain key of the queried model, matched by
// column alone or with the queried table, or a plain key of a joined table,
// matched with its table.
func (a *selector[M, R]) groupsBy(table, column string, shape projectionShape) bool {
	for _, key := range shape.keys {
		if key.Column != column || key.Bucket != types.TimeBucketNone {
			continue
		}
		keyTable := key.Table
		if len(keyTable) == 0 {
			keyTable = shape.main
		}
		if keyTable == table {
			return true
		}
	}
	return false
}

// derivedExpr renders a term a joined select projects: the derived table's
// column of the term's alias.
func (a *selector[M, R]) derivedExpr(jt *joinedTable, t types.Term) string {
	return a.db.quoteTableColumn(jt.alias, a.alias(t))
}

// whereScope is the scope the select's own predicates render in: the queried
// model's, and when the select joins, widened to the joined tables with every
// column qualified, because a name may exist on both sides.
func (a *selector[M, R]) whereScope(shape projectionShape) filterScope {
	scope := a.db.outerScope()
	if len(shape.joins) == 0 {
		return scope
	}
	scope.qualify = shape.main
	scope.columns = shape.mainInfo.columns
	scope.tables = shape.tables
	return scope
}

// columnOf resolves the column a term or an ordering names: on the queried
// model when it carries no table or the queried table, on a joined model
// when it carries that model's table, and nowhere otherwise — a reference of
// another model may well name a column the queried model also has, which is
// valid SQL over the wrong table, so the table is checked before the name.
func (a *selector[M, R]) columnOf(table, column string, shape projectionShape) (modelschema.Column, error) {
	columns := shape.columns
	if len(table) > 0 && table != shape.main {
		jt, ok := shape.joined[table]
		if !ok {
			return modelschema.Column{}, errors.Wrapf(ErrColumnTable, "%q belongs to table %q, the select reads %q", column, table, shape.main)
		}
		if jt.sub != nil {
			return modelschema.Column{}, errors.Wrapf(ErrJoinSelectColumn, "%q of %q", column, table)
		}
		columns = jt.columns
	}
	c, ok := columns[column]
	if !ok {
		return modelschema.Column{}, errors.Wrapf(ErrUnknownColumn, "%q", column)
	}
	return c, nil
}

// columnExpr renders a column of the select: bare when one table is read,
// which keeps every select without a join rendering as it always has, and
// qualified by its table when the select joins, the term's own or the
// queried model's, under the name that table is read by.
func (a *selector[M, R]) columnExpr(table, column string, shape projectionShape) string {
	if len(shape.joins) == 0 {
		return a.db.quoteIdent(column)
	}
	if len(table) == 0 {
		table = shape.main
	}
	if jt, joined := shape.joined[table]; joined {
		return a.db.quoteTableColumn(jt.alias, column)
	}
	return a.db.quoteTableColumn(table, column)
}

// joinedMeasureAllowed reports whether a measure may read a joined column in
// a grouped projection: only the aggregates that answer the same however
// many rows of a group share the joined row.
func joinedMeasureAllowed(fn types.TermFn) bool {
	switch fn {
	case types.FnMin, types.FnMax, types.FnCountDistinct:
		return true
	default:
		return false
	}
}
