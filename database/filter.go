package database

import (
	"fmt"
	"reflect"

	"github.com/cockroachdb/errors"

	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This file holds the predicate engine: the single implementation that turns a
// types.Filter tree into SQL. It is deliberately separate from the chainable
// options in query_options.go, because a rendered predicate is no longer tied
// to the WHERE clause of a list query. The same rules serve three call sites --
// the WHERE clause, the CASE guard of a conditional aggregate, and HAVING --
// and keeping one implementation is what stops the fail-closed behavior from
// drifting apart between them.

// filterScope is what the surrounding query means for the filters being
// rendered. It travels with the renderer because a filter list can appear at
// three depths -- the top-level WHERE, a conditional aggregate's CASE guard,
// and inside a correlated subquery -- and each answers "which table does this
// column belong to" differently.
type filterScope struct {
	// qualify prefixes column names. It is empty at the top level, where an
	// unqualified name is unambiguous, and set inside a subquery, where the
	// same name usually exists on both tables and an unqualified one silently
	// binds to the outer query instead.
	qualify string
	// parent is the table a correlated subquery joins back to.
	parent string
	// columns restricts which columns may be named, keyed by database name. It
	// is nil at the top level, where the producer already owns validation, and
	// set inside a subquery, where a name the related model does not have would
	// otherwise resolve against the outer table and quietly change the meaning
	// of the query.
	columns map[string]struct{}
	// timeColumns names the columns of the scope's model that store time,
	// keyed by database name. Comparisons on them normalize both sides through
	// timeComparableExpr; see comparisonSQL.
	timeColumns map[string]struct{}
	// depth numbers the nesting level so each subquery can take a distinct
	// alias when it reads the same table as the query enclosing it.
	depth int
}

// outerScope is the filterScope of a top-level predicate: it correlates
// against the chain's own table and knows which of that model's columns store
// time.
func (db *database[M]) outerScope() filterScope {
	return filterScope{parent: db.outerTableName(), timeColumns: timeColumnSet(reflect.TypeOf(*new(M)))}
}

// timeColumnSet reports the time-typed columns of a model type by database
// name, for the comparison normalization in comparisonSQL. A type whose
// columns cannot be resolved yields an empty set: its comparisons then render
// without normalization, which is the exact SQL every column renders on the
// dialects that compare time natively.
func timeColumnSet(typ reflect.Type) map[string]struct{} {
	columns, err := modelschema.Columns(typ)
	if err != nil {
		return nil
	}
	set := make(map[string]struct{})
	for _, c := range columns {
		if modelschema.ClassifyColumn(c.Type) == modelschema.ColumnClassTime {
			set[c.DBName] = struct{}{}
		}
	}
	return set
}

// applyFilters appends field-level operator filters, each as an AND
// condition with its value bound as a statement parameter. Every mismatch
// fails closed with "1 = 0" instead of being dropped: silently dropping a
// filter would widen the result set, which is dangerous when the result
// feeds deletes or exports. That covers an empty column, an operator the
// switch does not recognize, and a value whose type does not match the
// operator (see the Filter type for the per-operator value contract).
//
// The caller must hold db.mu.
func (db *database[M]) applyFilters(filters []types.Filter) {
	// The reason is discarded here on purpose: a client filter that cannot be
	// applied narrows the query instead of failing the request. Server-built
	// callers such as the aggregate builder read it and fail fast instead.
	if expr, _ := db.renderFilters(filters, false, db.outerScope()); expr != nil {
		db.ins = db.ins.Where(expr)
	}
}

// renderFilters turns a filter list into one composable predicate rather than
// applying it to a query directly. Returning an expression is what lets a
// single implementation serve every place a predicate appears — the WHERE
// clause, the CASE guard of a conditional aggregate, and HAVING — so the
// fail-closed rules cannot drift apart between them.
//
// The filters are OR-combined with each other when or is set and AND-combined
// otherwise; the top-level call always combines with AND. gorm parenthesizes
// the nesting as it builds the expression, so a group can never absorb the
// conditions around it.
//
// An empty list yields nil, which callers read as "no condition". An empty
// group fails closed instead; see groupCondition.
//
// The caller must hold db.mu.
func (db *database[M]) renderFilters(filters []types.Filter, or bool, scope filterScope) (clause.Expression, error) {
	if len(filters) == 0 {
		// No filters is not a failure: callers read a nil expression as "add no
		// condition". A sentinel here would make every call site branch on an
		// error that never means anything went wrong.
		return nil, nil //nolint:nilnil
	}
	var failure error
	exprs := make([]clause.Expression, 0, len(filters))
	for _, f := range filters {
		expr, err := db.renderFilter(f, scope)
		if err != nil && failure == nil {
			failure = err
		}
		exprs = append(exprs, expr)
	}
	if or {
		// A one-element OR group must never be handed to gorm as an
		// OrConditions: buildExprs reads that shape as an OR *connector* and
		// joins it to the preceding condition with OR, which turns a mandatory
		// sibling such as a tenant filter into an alternative and silently
		// widens the query. One alternative is just that condition, so it is
		// returned bare and the caller AND-combines it like any other.
		//
		// This cannot recurse into the same shape: renderFilters is the only
		// producer of OR groups, so a nested group has already been collapsed
		// by this rule or carries more than one child, which gorm renders
		// parenthesized and correctly.
		if len(exprs) == 1 {
			return exprs[0], failure
		}
		return clause.Or(exprs...), failure
	}
	return clause.And(exprs...), failure
}

// renderFilter turns one filter into a predicate.
//
// The caller must hold db.mu.
func (db *database[M]) renderFilter(f types.Filter, scope filterScope) (clause.Expression, error) {
	// Groups carry their children in Value and subqueries carry their columns
	// inside it, so neither names a column of its own and both are dispatched
	// before the empty-column check below.
	switch f.Op {
	case types.FilterOpOr:
		return db.groupCondition(f, true, scope)
	case types.FilterOpAnd:
		return db.groupCondition(f, false, scope)
	case types.FilterOpExists:
		sq, ok := f.Value.(types.Subquery)
		if !ok {
			return db.failClosedFilter(f, "expects a subquery value")
		}
		return db.existsCondition(f, sq, scope)
	}
	if len(f.Column) == 0 {
		return db.failClosedFilter(f, "has an empty column")
	}
	// Inside a subquery a name the related model does not have is not a typo
	// the database rejects: it resolves against the enclosing query instead and
	// turns the condition into a correlated reference, which is valid SQL over
	// the wrong rows.
	if scope.columns != nil {
		if _, ok := scope.columns[f.Column]; !ok {
			return db.failClosedFilter(f, "names a column the related model does not have")
		}
	}
	column := db.scopedColumn(f.Column, scope)
	switch f.Op {
	case types.FilterOpEq:
		return db.scalarFilter(f, db.comparisonSQL(scope, f.Column, column, " = "))
	case types.FilterOpNe:
		return db.scalarFilter(f, db.comparisonSQL(scope, f.Column, column, " <> "))
	case types.FilterOpGt:
		return db.scalarFilter(f, db.comparisonSQL(scope, f.Column, column, " > "))
	case types.FilterOpGte:
		return db.scalarFilter(f, db.comparisonSQL(scope, f.Column, column, " >= "))
	case types.FilterOpLt:
		return db.scalarFilter(f, db.comparisonSQL(scope, f.Column, column, " < "))
	case types.FilterOpLte:
		return db.scalarFilter(f, db.comparisonSQL(scope, f.Column, column, " <= "))
	case types.FilterOpIn:
		return db.listFilter(f, column+" IN ?")
	case types.FilterOpNotIn:
		return db.listFilter(f, column+" NOT IN ?")
	case types.FilterOpLike:
		return db.patternFilter(f, column+" LIKE ?"+likeEscapeClause, "%", "%")
	case types.FilterOpNotLike:
		return db.patternFilter(f, column+" NOT LIKE ?"+likeEscapeClause, "%", "%")
	case types.FilterOpStartsWith:
		return db.patternFilter(f, column+" LIKE ?"+likeEscapeClause, "", "%")
	case types.FilterOpEndsWith:
		return db.patternFilter(f, column+" LIKE ?"+likeEscapeClause, "%", "")
	case types.FilterOpIsNull:
		b, ok := f.Value.(bool)
		if !ok {
			return db.failClosedFilter(f, "expects a bool value")
		}
		if b {
			return clause.Expr{SQL: column + " IS NULL"}, nil
		}
		return clause.Expr{SQL: column + " IS NOT NULL"}, nil
	case types.FilterOpRegex:
		return db.stringFilter(f, column+" "+db.regexpOperator()+" ?")
	case types.FilterOpNotRegex:
		return db.stringFilter(f, "NOT ("+column+" "+db.regexpOperator()+" ?)")
	case types.FilterOpJSONContains:
		// datatypes handles the dialect split: JSON_CONTAINS on MySQL and
		// a json_each EXISTS subquery on SQLite. The column is passed
		// unquoted because the expression quotes it itself.
		s, ok := f.Value.(string)
		if !ok {
			return db.failClosedFilter(f, "expects a string value")
		}
		return datatypes.JSONArrayQuery(f.Column).Contains(s), nil
	default:
		return db.failClosedFilter(f, "is unknown")
	}
}

// groupCondition builds one filter group as a single nested predicate, so the
// conditions outside the group can never be absorbed into it. Its children are
// OR-combined when or is set and AND-combined otherwise; a child may itself be
// a group, which is how arbitrary nesting works.
//
// A group whose value is not a filter list, or that carries no children at
// all, fails closed: an empty group is a caller bug, and answering it with the
// logical identity (TRUE for AND) would widen the result set.
func (db *database[M]) groupCondition(f types.Filter, or bool, scope filterScope) (clause.Expression, error) {
	children, ok := f.Value.([]types.Filter)
	if !ok {
		return db.failClosedFilter(f, "expects a filter list value")
	}
	if len(children) == 0 {
		return db.failClosedFilter(f, "has no children")
	}
	return db.renderFilters(children, or, scope)
}

// failClosedExpr is the predicate that matches nothing. Narrowing to an empty
// result is always safe; widening it is not.
func failClosedExpr() clause.Expression { return clause.Expr{SQL: "1 = 0"} }

// failClosedFilter records why a filter cannot be applied and narrows the
// query to an empty result instead of widening it.
func (db *database[M]) failClosedFilter(f types.Filter, msg string) (clause.Expression, error) {
	logger.Database.WithContext(db.ctx, consts.Phase("WithQuery")).Warnf("filter operator %q on column %q %s, adding safety condition", f.Op, f.Column, msg)
	return failClosedExpr(), errors.Wrapf(ErrUnusableFilter, "operator %q on column %q %s", f.Op, f.Column, msg)
}

// comparisonSQL renders "column op ?" for one comparison filter. A column the
// scope knows to store time takes both sides through timeComparableExpr, so
// the comparison agrees across storage spellings; every other column renders
// the plain comparison.
func (db *database[M]) comparisonSQL(scope filterScope, dbName, quotedColumn, op string) string {
	if _, isTime := scope.timeColumns[dbName]; isTime {
		return db.timeComparableExpr(quotedColumn) + op + db.timeComparableExpr("?")
	}
	return quotedColumn + op + "?"
}

// scalarFilter binds a comparison filter whose value must be a scalar; nil,
// slice, and array values fail closed.
func (db *database[M]) scalarFilter(f types.Filter, sql string) (clause.Expression, error) {
	if f.Value == nil {
		return db.failClosedFilter(f, "expects a scalar value")
	}
	if k := reflect.ValueOf(f.Value).Kind(); k == reflect.Slice || k == reflect.Array {
		return db.failClosedFilter(f, "expects a scalar value")
	}
	return clause.Expr{SQL: sql, Vars: []any{f.Value}}, nil
}

// listFilter binds a set-membership filter whose value must be a slice or an
// array; anything else, including a comma-separated string, fails closed. An
// empty slice keeps the SQL list semantics: IN matches nothing, and the result
// never widens.
func (db *database[M]) listFilter(f types.Filter, sql string) (clause.Expression, error) {
	if f.Value == nil {
		return db.failClosedFilter(f, "expects a slice value")
	}
	if k := reflect.ValueOf(f.Value).Kind(); k != reflect.Slice && k != reflect.Array {
		return db.failClosedFilter(f, "expects a slice value")
	}
	return clause.Expr{SQL: sql, Vars: []any{f.Value}}, nil
}

// patternFilter binds a LIKE-family filter; the value must be a string and is
// escaped so the stored value matches literally.
func (db *database[M]) patternFilter(f types.Filter, sql, prefix, suffix string) (clause.Expression, error) {
	s, ok := f.Value.(string)
	if !ok {
		return db.failClosedFilter(f, "expects a string value")
	}
	return clause.Expr{SQL: sql, Vars: []any{prefix + escapeLikePattern(s) + suffix}}, nil
}

// stringFilter binds a filter whose value must be a plain string bound as-is
// (the regex operators).
func (db *database[M]) stringFilter(f types.Filter, sql string) (clause.Expression, error) {
	s, ok := f.Value.(string)
	if !ok {
		return db.failClosedFilter(f, "expects a string value")
	}
	return clause.Expr{SQL: sql, Vars: []any{s}}, nil
}

// scopedColumn renders a column name for the scope it is read in: qualified
// inside a subquery, bare at the top level where qualification would add noise
// without removing any ambiguity.
func (db *database[M]) scopedColumn(column string, scope filterScope) string {
	if len(scope.qualify) == 0 {
		return db.quoteIdent(column)
	}
	return db.quoteTableColumn(scope.qualify, column)
}

// existsCondition renders a correlated subquery as a semi join. The related
// model is attached with Model, so the subquery inherits that model's
// soft-delete scope: a subquery can never match a row a List on the related
// model hides.
//
// The correlation compares two qualified columns rather than binding a value,
// so it is written into the SQL; both sides are quoted identifiers taken from
// column references, which cannot carry SQL.
//
// scope carries the table the outer side of the correlation refers to. It
// travels with the renderer rather than being read from the chain, because a
// subquery may itself contain one: the inner correlation must reach the table
// directly enclosing it, and reading the chain would always yield the outermost
// model instead. That mistake produces valid SQL joined against the wrong
// table, which returns a wrong row set rather than an error.
//
// The caller must hold db.mu.
func (db *database[M]) existsCondition(f types.Filter, sq types.Subquery, scope filterScope) (clause.Expression, error) {
	if sq.Model == nil {
		return db.failClosedFilter(f, "has no related model")
	}
	if len(sq.ChildColumn) == 0 || len(sq.ParentColumn) == 0 {
		return db.failClosedFilter(f, "has an empty correlation column")
	}
	if len(scope.parent) == 0 {
		return db.failClosedFilter(f, "cannot resolve the table to correlate against")
	}
	childType := reflect.TypeOf(sq.Model)
	childTable := sq.Model.GetTableName()
	if len(childTable) == 0 {
		// A model only reports a table name when it overrides GetTableName;
		// otherwise gorm derives it, so the same resolution is used here.
		resolved, err := modelschema.TableName(childType)
		if err != nil {
			return db.failClosedFilter(f, "related model has no resolvable table name")
		}
		childTable = resolved
	}
	childColumns, err := modelschema.Columns(childType)
	if err != nil {
		return db.failClosedFilter(f, "related model has no resolvable columns")
	}
	allowed := make(map[string]struct{}, len(childColumns))
	childTimeColumns := make(map[string]struct{})
	for _, c := range childColumns {
		allowed[c.DBName] = struct{}{}
		if modelschema.ClassifyColumn(c.Type) == modelschema.ColumnClassTime {
			childTimeColumns[c.DBName] = struct{}{}
		}
	}
	if _, ok := allowed[sq.ChildColumn]; !ok {
		return db.failClosedFilter(f, "correlates on a column the related model does not have")
	}

	// A subquery reading the same table as the query around it needs its own
	// name, or both sides of the correlation resolve to the inner table and the
	// condition degenerates into a comparison of a row with itself. Aliasing
	// only that case keeps every other subquery rendering exactly as before.
	// Table takes a bare name: pre-quoting it makes gorm read the value as a
	// raw table expression, and the soft-delete clause then qualifies itself
	// with the struct-derived name instead of this one.
	from := childTable
	childRef := childTable
	if childTable == scope.parent {
		alias := fmt.Sprintf("%s_gst%d", childTable, scope.depth+1)
		from = childTable + " AS " + alias
		childRef = alias
	}

	correlation := db.quoteTableColumn(childRef, sq.ChildColumn) +
		" = " + db.quoteTableColumn(scope.parent, sq.ParentColumn)

	// Table is set explicitly alongside Model. Model alone would let gorm name
	// the FROM from the struct, and gorm reads its own TableName method rather
	// than the framework's GetTableName, so a model that overrides only the
	// latter would be selected FROM one table while the correlation qualifies
	// another.
	sub := db.ins.Session(&gorm.Session{NewDB: true}).
		Table(from).
		Model(sq.Model).
		Select("1").
		Where(correlation)
	// Nested filters read the subquery's own table, correlate back to it, and
	// may only name its columns.
	inner := filterScope{
		qualify:     childRef,
		parent:      childRef,
		columns:     allowed,
		timeColumns: childTimeColumns,
		depth:       scope.depth + 1,
	}
	expr, failure := db.renderFilters(sq.Filters, false, inner)
	if expr != nil {
		sub = sub.Where(expr)
	}
	// A predicate that could not be rendered fails closed to "match nothing".
	// Negating the subquery would turn that into "match everything", so the
	// whole condition collapses instead of the inner one: narrowing is always
	// safe, widening never is, and this is the only place in the renderer where
	// the difference is a negation away.
	if failure != nil {
		return failClosedExpr(), failure
	}
	if sq.Negate {
		return clause.Expr{SQL: "NOT EXISTS (?)", Vars: []any{sub}}, nil
	}
	return clause.Expr{SQL: "EXISTS (?)", Vars: []any{sub}}, nil
}

// outerTableName resolves the table the current chain reads, for qualifying
// the outer side of a correlation. It derives the name from M directly instead
// of from the prepared model, because filters are built while the chain is
// still being assembled, before the terminal operation prepares it.
func (db *database[M]) outerTableName() string {
	typ := reflect.TypeOf(*new(M))
	if typ == nil || typ.Kind() != reflect.Pointer {
		return ""
	}
	m, ok := reflect.New(typ.Elem()).Interface().(M)
	if !ok {
		return ""
	}
	if name := m.GetTableName(); len(name) > 0 {
		return name
	}
	name, err := modelschema.TableName(typ)
	if err != nil {
		return ""
	}
	return name
}
