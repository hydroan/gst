package database

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
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
	// outer is the table the scope's own EqCol predicates equate against:
	// the table of the query directly enclosing this subquery, under the name
	// it is read by, which is an alias when the subquery reads the same
	// table. It is empty at the top level, where an EqCol predicate has
	// nothing to tie to and fails closed. outerTable is that table's own
	// name, which the outer side of an EqCol must name when it carries one.
	outer      string
	outerTable string
	// outerColumns names the columns of the enclosing model, keyed by database
	// name, so the outer side of an EqCol predicate is checked the same way the inner
	// side is instead of reaching the database as an unknown column. It is nil
	// at the top level, alongside outer.
	outerColumns map[string]struct{}
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
	// jsonColumns names the columns of the scope's model that store a JSON
	// document, keyed by database name. The like-family filters match against
	// the document's text form and cast the column where the dialect requires
	// it; see textPatternColumn.
	jsonColumns map[string]struct{}
	// depth numbers the nesting level so each subquery can take a distinct
	// alias when it reads the same table as the query enclosing it.
	depth int
	// table is the table the scope's own columns belong to, unaliased. A
	// filter carrying another table is applied to that table when tables
	// lists it, and fails closed otherwise.
	table string
	// tables are the other tables a predicate may name inside a join, keyed
	// by table name, each with the columns it may name. It is nil outside a
	// join, where a predicate reads one table only.
	tables map[string]tableInfo
	// rename maps the scope's own columns to the names they are read under,
	// for the ON of a joined select; nil everywhere else.
	rename map[string]string
	// derived marks the ON scope of a joined select, whose rows are the
	// groups the select materialized: a subquery has no row of the select's
	// model to correlate with there.
	derived bool
}

// tableInfo is what the renderer knows about one table's columns: which
// exist, and which store time or JSON, keyed by database name. A derived
// table also knows the name it is read under and the aliases its key columns
// project as, which is how a column named through the joined select's model
// reaches the derived column it became.
type tableInfo struct {
	columns     map[string]struct{}
	timeColumns map[string]struct{}
	jsonColumns map[string]struct{}
	// qualify is the name the table's columns are qualified with in SQL;
	// empty means the table's own name.
	qualify string
	// rename maps a column name to the name it is read under, for a derived
	// table whose key columns project under aliases; nil for a model table.
	rename map[string]string
	// aliases lists what a derived table projects, in order, for the message
	// a column it does not have gets; nil for a model table.
	aliases []string
}

// column returns the name a column is read under in this table.
func (info tableInfo) column(name string) string {
	if alias, ok := info.rename[name]; ok {
		return alias
	}
	return name
}

// tableInfoOf reads a model's resolved columns into a tableInfo.
func tableInfoOf(columns []modelschema.Column) tableInfo {
	info := tableInfo{
		columns:     make(map[string]struct{}, len(columns)),
		timeColumns: make(map[string]struct{}),
		jsonColumns: make(map[string]struct{}),
	}
	for _, c := range columns {
		info.columns[c.DBName] = struct{}{}
		if modelschema.ClassifyColumn(c.Type) == modelschema.ColumnClassTime {
			info.timeColumns[c.DBName] = struct{}{}
		}
		if modelschema.IsJSONType(c.Type) {
			info.jsonColumns[c.DBName] = struct{}{}
		}
	}
	return info
}

// own is the scope's own table as a tableInfo.
func (s filterScope) own() tableInfo {
	return tableInfo{columns: s.columns, timeColumns: s.timeColumns, jsonColumns: s.jsonColumns, qualify: s.qualify, rename: s.rename}
}

// outerScope is the filterScope of a top-level predicate: it correlates
// against the chain's own table and knows which of that model's columns store
// time and which store JSON.
func (db *database[M]) outerScope() filterScope {
	typ := reflect.TypeOf(*new(M))
	return filterScope{
		parent:      db.outerTableName(),
		table:       db.outerTableName(),
		timeColumns: modelschema.TimeColumnSet(typ),
		jsonColumns: modelschema.JSONColumnSet(typ),
	}
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
	// A filter of another model's column is not client input: only a column
	// reference carries a table, and service code wrote it, so it fails the
	// chain the way WithSelect refuses the same reference. Every other
	// mismatch is left to fail closed, its reason discarded on purpose: a
	// client filter that cannot be applied narrows the query instead of
	// failing the request. Server-built callers such as the select builder
	// read the reason and fail fast instead.
	if f, reason, foreign := db.foreignTableFilter(filters, db.outerTableName(), ""); foreign {
		db.err = errors.Wrapf(ErrColumnTable, "filter %q on column %q %s", f.Op, f.Column, reason)
		return
	}
	if expr, _ := db.renderFilters(filters, false, db.outerScope()); expr != nil {
		db.ins = db.ins.Where(expr)
	}
}

// foreignTableFilter finds a filter that names a column of a table its scope
// does not read, with the reason it is refused for: looking through the OR
// and AND groups and into the subqueries, where a filter names the related
// model's table, own, and the outer side of an EqCol the table enclosing the
// subquery. At the top level there is no enclosing table, and an EqCol there
// is the renderer's to refuse. The reason names the reader that refused the
// column: the chain's model at the top level, the subquery below it, which
// may well read the same table.
func (db *database[M]) foreignTableFilter(filters []types.Filter, own, enclosing string) (found types.Filter, reason string, foreign bool) {
	reader := fmt.Sprintf("model %s reading %q", reflect.TypeOf(*new(M)).Elem().Name(), own)
	if len(enclosing) > 0 {
		reader = fmt.Sprintf("the subquery over %q", own)
	}
	for _, f := range filters {
		switch f.Op {
		case types.FilterOpOr, types.FilterOpAnd:
			if children, ok := f.Value.([]types.Filter); ok {
				if found, reason, foreign = db.foreignTableFilter(children, own, enclosing); foreign {
					return found, reason, true
				}
			}
		case types.FilterOpExists:
			if sq, ok := f.Value.(types.Subquery); ok && sq.Model != nil {
				if found, reason, foreign = db.foreignTableFilter(sq.Filters, sq.Model.TableName(), own); foreign {
					return found, reason, true
				}
			}
		case types.FilterOpEqCol:
			if len(f.Table) > 0 && f.Table != own {
				return f, fmt.Sprintf("names table %q, which %s does not read", f.Table, reader), true
			}
			if _, parentTable, ok := eqColParent(f.Value); ok && len(enclosing) > 0 && len(parentTable) > 0 && parentTable != enclosing {
				return f, fmt.Sprintf("ties to a column of %q, which is not the table enclosing %s", parentTable, reader), true
			}
		default:
			if len(f.Table) > 0 && f.Table != own {
				return f, fmt.Sprintf("names table %q, which %s does not read", f.Table, reader), true
			}
		}
	}
	return types.Filter{}, "", false
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
	case types.FilterOpFalse:
		// The caller asked for the predicate that matches nothing, so it
		// renders without the warning a filter that cannot be applied logs.
		return falseExpr(), nil
	}
	if len(f.Column) == 0 {
		return db.failClosedFilter(f, "has an empty column")
	}
	column, info, err := db.placeFilter(f, scope)
	if err != nil {
		return falseExpr(), err
	}
	switch f.Op {
	case types.FilterOpEq:
		return db.scalarFilter(f, db.comparisonSQL(info, f.Column, column, " = "))
	case types.FilterOpNe:
		return db.scalarFilter(f, db.comparisonSQL(info, f.Column, column, " <> "))
	case types.FilterOpGt:
		return db.scalarFilter(f, db.comparisonSQL(info, f.Column, column, " > "))
	case types.FilterOpGte:
		return db.scalarFilter(f, db.comparisonSQL(info, f.Column, column, " >= "))
	case types.FilterOpLt:
		return db.scalarFilter(f, db.comparisonSQL(info, f.Column, column, " < "))
	case types.FilterOpLte:
		return db.scalarFilter(f, db.comparisonSQL(info, f.Column, column, " <= "))
	case types.FilterOpIn:
		return db.listFilter(f, column+" IN ?")
	case types.FilterOpNotIn:
		return db.listFilter(f, column+" NOT IN ?")
	case types.FilterOpLike:
		return db.patternFilter(f, db.likeColumn(info, f.Column, column)+" LIKE ?"+db.likeEscapeSuffix(), "%", "%")
	case types.FilterOpNotLike:
		return db.patternFilter(f, db.likeColumn(info, f.Column, column)+" NOT LIKE ?"+db.likeEscapeSuffix(), "%", "%")
	case types.FilterOpStartsWith:
		return db.patternFilter(f, db.likeColumn(info, f.Column, column)+" LIKE ?"+db.likeEscapeSuffix(), "", "%")
	case types.FilterOpEndsWith:
		return db.patternFilter(f, db.likeColumn(info, f.Column, column)+" LIKE ?"+db.likeEscapeSuffix(), "%", "")
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
		// datatypes handles the dialect split: JSON_CONTAINS on MySQL, a
		// jsonb operator on Postgres, a json_each EXISTS subquery on SQLite.
		// It covers only those three — on any other dialect its expression
		// renders empty, which would silently WIDEN the result, so the filter
		// fails closed there instead. The column is passed unquoted, under
		// the table it was placed in, because the expression quotes it
		// itself, every dialect's quoting spelling table.column as two
		// identifiers.
		switch db.dialect() {
		case dialectMySQL, dialectPostgres, dialectSQLite:
		default:
			return db.failClosedFilter(f, "is not supported on this dialect")
		}
		s, ok := f.Value.(string)
		if !ok {
			return db.failClosedFilter(f, "expects a string value")
		}
		return datatypes.JSONArrayQuery(db.placedName(f, scope, info)).Contains(s), nil
	case types.FilterOpEqCol:
		return db.eqColCondition(f, column, scope)
	default:
		return db.failClosedFilter(f, "is unknown")
	}
}

// placeFilter resolves the table a filter reads: the scope's own table when
// the filter carries none or names it, one of the tables read beside it when
// the query joins that table, and nothing otherwise, which fails closed like
// an unknown column would. It returns the column quoted for that table and
// what the renderer knows about the table's columns.
//
// Inside a subquery a name the related model does not have is not a typo the
// database rejects: it resolves against the enclosing query instead and turns
// the condition into a correlated reference, which is valid SQL over the
// wrong rows. The scope's own columns are therefore checked wherever the
// scope lists them.
func (db *database[M]) placeFilter(f types.Filter, scope filterScope) (string, tableInfo, error) {
	if len(f.Table) > 0 && len(scope.table) > 0 && f.Table != scope.table {
		info, ok := scope.tables[f.Table]
		if !ok {
			// Joined with the column-table sentinel as well, so a caller
			// matching either sees the same mistake List reports.
			_, err := db.failClosedFilter(f, fmt.Sprintf("names a column of table %q, which the query does not read", f.Table))
			return "", tableInfo{}, errors.Join(err, ErrColumnTable)
		}
		if _, ok := info.columns[f.Column]; !ok {
			if info.rename != nil {
				// A derived table has the columns the joined select projects,
				// and a filter names one of its keys; a condition on the
				// select's rows narrows the select itself.
				_, err := db.failClosedFilter(f, fmt.Sprintf("names %q, which is not a key of the joined select over %q; the select projects %s, and a condition on its rows belongs to its own Where or Having", f.Column, f.Table, strings.Join(info.aliases, ", ")))
				return "", tableInfo{}, errors.Join(err, ErrJoinSelectColumn)
			}
			_, err := db.failClosedFilter(f, "names a column its table does not have")
			return "", tableInfo{}, err
		}
		return db.tableColumn(f.Table, info, f.Column), info, nil
	}
	if scope.columns != nil {
		if _, ok := scope.columns[f.Column]; !ok {
			_, err := db.failClosedFilter(f, fmt.Sprintf("names a column %q does not have", scope.table))
			return "", tableInfo{}, err
		}
	}
	return db.scopedColumn(f.Column, scope), scope.own(), nil
}

// placedName spells a placed column unquoted, under the table placeFilter
// placed it in, for an expression that quotes its operand itself: bare at
// the top level of a single-table read, table.column wherever the renderer
// qualifies.
func (db *database[M]) placedName(f types.Filter, scope filterScope, info tableInfo) string {
	qualify := info.qualify
	if len(qualify) == 0 && len(f.Table) > 0 && len(scope.table) > 0 && f.Table != scope.table {
		qualify = f.Table
	}
	name := info.column(f.Column)
	if len(qualify) == 0 {
		return name
	}
	return qualify + "." + name
}

// eqColCondition renders an EqCol predicate. Inside a subquery it ties the
// scope's own column, on the left, to the enclosing query's column on the
// right; outside a subquery there is an enclosing query only when the select
// joins, where the predicate ties two tables of the query, and it fails
// closed everywhere else rather than comparing a table with itself. The
// column arrives placed by the caller, so inside an aliased self join it
// already names the alias. The other column must belong to the table it
// names: an unchecked side would surface as a database error instead of the
// fail-closed answer every other mistake gets.
//
// The caller must hold db.mu.
func (db *database[M]) eqColCondition(f types.Filter, column string, scope filterScope) (clause.Expression, error) {
	parent, parentTable, ok := eqColParent(f.Value)
	if !ok {
		return db.failClosedFilter(f, "expects a column name or a column reference value")
	}
	if len(parent) == 0 {
		return db.failClosedFilter(f, "has an empty correlation column")
	}
	if scope.tables != nil && len(scope.outer) == 0 {
		return db.joinEqCol(f, column, parent, parentTable, scope)
	}
	if len(scope.outer) == 0 {
		return db.failClosedFilter(f, "correlates outside a subquery")
	}
	if scope.outerColumns == nil {
		return db.failClosedFilter(f, "cannot resolve the enclosing model's columns")
	}
	// A reference names its table; the enclosing model may well have a
	// column of the same name, and the name alone would tie the subquery to
	// it as valid SQL over the wrong rows.
	if len(parentTable) > 0 && parentTable != scope.outerTable {
		expr, err := db.failClosedFilter(f, fmt.Sprintf("ties to a column of %q, which is not the enclosing table %q", parentTable, scope.outerTable))
		return expr, errors.Join(err, ErrColumnTable)
	}
	if _, ok := scope.outerColumns[parent]; !ok {
		return db.failClosedFilter(f, "correlates on a column the enclosing model does not have")
	}
	return clause.Expr{SQL: column + " = " + db.quoteTableColumn(scope.outer, parent)}, nil
}

// joinEqCol renders an EqCol predicate inside a join: the placed column on
// one side, the other column qualified by its own table, which must be the
// scope's own table or one read beside it. The other side has to carry its
// table, which only a column reference does: a plain name could belong to
// either table.
//
// The caller must hold db.mu.
func (db *database[M]) joinEqCol(f types.Filter, column, parent, parentTable string, scope filterScope) (clause.Expression, error) {
	if len(parentTable) == 0 {
		return db.failClosedFilter(f, "ties to a column without a table, name it through a column reference")
	}
	var info tableInfo
	switch parentTable {
	case scope.table:
		info = scope.own()
	default:
		other, ok := scope.tables[parentTable]
		if !ok {
			// Joined with the column-table sentinel as well, the way a
			// filter naming such a table is refused.
			expr, err := db.failClosedFilter(f, fmt.Sprintf("ties to a column of table %q, which the query does not read", parentTable))
			return expr, errors.Join(err, ErrColumnTable)
		}
		info = other
	}
	if _, ok := info.columns[parent]; !ok {
		return db.failClosedFilter(f, "ties to a column its table does not have")
	}
	return clause.Expr{SQL: column + " = " + db.tableColumn(parentTable, info, parent)}, nil
}

// tableColumn renders a column of a table in scope, under the name the table
// is read by and the name the column is read under.
func (db *database[M]) tableColumn(table string, info tableInfo, column string) string {
	qualify := info.qualify
	if len(qualify) == 0 {
		qualify = table
	}
	return db.quoteTableColumn(qualify, info.column(column))
}

// eqColParent reads the other column of an EqCol predicate: a plain name,
// which carries no table, or a column reference, which carries its own.
func eqColParent(value any) (name, table string, ok bool) {
	switch v := value.(type) {
	case string:
		return v, "", true
	case types.AnyColumnRef:
		return v.Name(), v.Table(), true
	default:
		return "", "", false
	}
}

// hasCorrelation reports whether a subquery's predicates tie it to the query
// around it: an EqCol predicate directly in the list or inside a group, at this level
// only. A nested subquery correlates against this level, not on its behalf,
// so its own predicates do not count.
func hasCorrelation(filters []types.Filter) bool {
	for _, f := range filters {
		switch f.Op {
		case types.FilterOpEqCol:
			return true
		case types.FilterOpOr, types.FilterOpAnd:
			if children, ok := f.Value.([]types.Filter); ok && hasCorrelation(children) {
				return true
			}
		}
	}
	return false
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

// falseExpr is the predicate that matches nothing: what FilterFalse renders
// as on purpose, and what every fail-closed answer narrows a query to.
// Narrowing to an empty result is always safe; widening it is not.
func falseExpr() clause.Expression { return clause.Expr{SQL: "1 = 0"} }

// failClosedFilter records why a filter cannot be applied and narrows the
// query to an empty result instead of widening it.
func (db *database[M]) failClosedFilter(f types.Filter, msg string) (clause.Expression, error) {
	logger.Database.WithContext(db.ctx, phaseWithQuery).Warnz(
		"filter cannot be applied, adding safety condition",
		zap.String("op", string(f.Op)),
		zap.String("column", f.Column),
		zap.String("reason", msg),
	)
	return falseExpr(), errors.Wrapf(ErrUnusableFilter, "operator %q on column %q %s", f.Op, f.Column, msg)
}

// comparisonSQL renders "column op ?" for one comparison filter. A column its
// table is known to store time in takes both sides through
// timeComparableExpr, so the comparison agrees across storage spellings;
// every other column renders the plain comparison.
func (db *database[M]) comparisonSQL(info tableInfo, dbName, quotedColumn, op string) string {
	if _, isTime := info.timeColumns[dbName]; isTime {
		return db.timeComparableExpr(quotedColumn) + op + db.timeComparableExpr("?")
	}
	return quotedColumn + op + "?"
}

// likeColumn renders the column a like-family filter matches against: a JSON
// document matches by its text form, so a JSON column goes through
// textPatternColumn and is cast where the dialect requires it.
func (db *database[M]) likeColumn(info tableInfo, dbName, quotedColumn string) string {
	_, isJSON := info.jsonColumns[dbName]
	return db.textPatternColumn(quotedColumn, isJSON)
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
	return clause.Expr{SQL: sql, Vars: []any{prefix + db.escapeLikePattern(s) + suffix}}, nil
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
		return db.quoteIdent(scope.own().column(column))
	}
	return db.quoteTableColumn(scope.qualify, scope.own().column(column))
}

// existsCondition renders a correlated subquery as a semi join. The related
// model is attached with Model, so the subquery inherits that model's
// soft-delete scope: a subquery can never match a row a List on the related
// model hides.
//
// The correlation predicates compare two qualified columns rather than binding
// a value, so they are written into the SQL as identifiers quoted by the
// dialect (see eqColCondition): the related side is checked against the
// related model's columns, and both sides are named by service code — through
// a column reference or a plain name — never by a client, because the operator
// has no URL spelling.
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
	// ClickHouse cannot resolve a correlated column from the enclosing query
	// (only constants and CTEs cross that scope boundary), so the semi join
	// fails closed there rather than failing the whole statement.
	if db.dialect() == dialectClickHouse {
		return db.failClosedFilter(f, "is not supported on this dialect")
	}
	if sq.Model == nil {
		return db.failClosedFilter(f, "has no related model")
	}
	if !hasCorrelation(sq.Filters) {
		return db.failClosedFilter(f, "has no correlation")
	}
	if scope.derived {
		// The ON of a joined select ties the query to the select's groups,
		// which are no rows of the select's model a subquery could correlate
		// with; the select's own Where narrows the rows it groups.
		return db.failClosedFilter(f, "correlates inside the ON of a joined select, whose rows are its groups; narrow the select with its own Where instead")
	}
	if len(scope.parent) == 0 {
		return db.failClosedFilter(f, "cannot resolve the table to correlate against")
	}
	childType := reflect.TypeOf(sq.Model)
	childTable := sq.Model.TableName()
	if len(childTable) == 0 {
		// Every model must declare its table name; a missing declaration
		// fails the predicate closed instead of flowing an empty table name
		// into SQL.
		return db.failClosedFilter(f, "related model declares no table name")
	}
	childColumns, err := modelschema.Columns(childType)
	if err != nil {
		return db.failClosedFilter(f, "related model has no resolvable columns")
	}
	child := tableInfoOf(childColumns)

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

	// Table is set explicitly alongside Model: the predicates, correlations
	// included, qualify columns with childRef, so the FROM clause must carry
	// the same name — including its aliased form — rather than the bare name
	// gorm would take from the struct.
	sub := db.ins.Session(&gorm.Session{NewDB: true}).
		Table(from).
		Model(sq.Model).
		Select("1")
	// The predicates read the subquery's own table and may only name its
	// columns; their EqCol predicates equate against the table this
	// subquery hangs off, while a subquery nested one level further down
	// correlates back to this one. The enclosing model's columns come from the
	// scope around this subquery when it is itself a subquery, and from the
	// queried model at the first level.
	outerColumns := scope.columns
	if outerColumns == nil {
		outerColumns = db.outerColumnSet()
	}
	inner := filterScope{
		qualify:      childRef,
		parent:       childRef,
		outer:        scope.parent,
		outerTable:   scope.table,
		outerColumns: outerColumns,
		columns:      child.columns,
		timeColumns:  child.timeColumns,
		jsonColumns:  child.jsonColumns,
		depth:        scope.depth + 1,
		table:        childTable,
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
		return falseExpr(), failure
	}
	if sq.Negate {
		return clause.Expr{SQL: "NOT EXISTS (?)", Vars: []any{sub}}, nil
	}
	return clause.Expr{SQL: "EXISTS (?)", Vars: []any{sub}}, nil
}

// outerColumnSet lists the columns of the model the current chain reads, keyed
// by database name, for checking the outer side of a first-level correlation.
// It resolves M through the same cached schema parse the related model goes
// through; a model whose columns cannot be resolved yields nil, which the
// correlation fails closed on.
func (db *database[M]) outerColumnSet() map[string]struct{} {
	typ := reflect.TypeOf(*new(M))
	if typ == nil {
		return nil
	}
	columns, err := modelschema.Columns(typ)
	if err != nil {
		return nil
	}
	set := make(map[string]struct{}, len(columns))
	for _, c := range columns {
		set[c.DBName] = struct{}{}
	}
	return set
}

// outerTableName resolves the table the current chain reads, for qualifying
// the outer side of a correlation. It reads the name from M directly instead
// of from the prepared model, because filters are built while the chain is
// still being assembled, before the terminal operation prepares it. A model
// without an explicit table name yields "", which the caller fails closed.
func (db *database[M]) outerTableName() string {
	typ := reflect.TypeOf(*new(M))
	if typ == nil || typ.Kind() != reflect.Pointer {
		return ""
	}
	m, ok := reflect.TypeAssert[M](reflect.New(typ.Elem()))
	if !ok {
		return ""
	}
	return m.TableName()
}
