package database

import (
	"context"
	"reflect"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Errors reported while an aggregate query is built. They all fail fast: an
// aggregate projection is written by service code, not parsed from a request,
// so a mistake in it is a programming error. Answering it with an empty result
// the way the filter layer answers a malformed client filter would disguise
// the bug as "no data today", which is the hardest reporting failure to trace.
var (
	ErrEmptyProjection    = errors.New("aggregate projection is empty")
	ErrNoAggregateFn      = errors.New("aggregate projection declares no aggregate function, use List for a plain read")
	ErrInvalidAlias       = errors.New("aggregate alias is not a valid identifier")
	ErrDuplicateAlias     = errors.New("aggregate alias is declared twice")
	ErrUnknownColumn      = errors.New("aggregate names a column the model does not have")
	ErrAggregateType      = errors.New("aggregate function does not accept this column type")
	ErrResultFieldMissing = errors.New("result row has no field for aggregate alias")
	ErrAliasMissing       = errors.New("aggregate projection has no alias for result row field")
	ErrGroupedScanOne     = errors.New("ScanOne cannot run a grouped aggregation, use Scan")
)

// aliasPattern is what an alias must look like. An alias reaches SQL as an
// identifier rather than a bound value, so it is restricted to a plain
// identifier instead of being quoted and hoped for.
var aliasPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// aggregator implements types.Aggregator by borrowing the Database chain for
// everything an analytical read shares with a plain one: the transaction
// carried by the context, identifier quoting, the filter renderer, tracing and
// SQL collection.
type aggregator[M types.Model, R any] struct {
	db *database[M]

	terms    []types.AggregateTerm
	filters  []types.Filter
	havings  []types.Having
	orders   []types.AggregateOrder
	limit    int
	offset   int
	hasLimit bool
}

// Aggregate creates an analytical read over the table of M whose result rows
// scan into R. See types.Aggregator for the contract and an example.
func Aggregate[M types.Model, R any](ctx context.Context) types.Aggregator[M, R] {
	inner, _ := Database[M](ctx).(*database[M])
	return &aggregator[M, R]{db: inner}
}

func (a *aggregator[M, R]) Select(terms ...types.AggregateTerm) types.Aggregator[M, R] {
	a.terms = append(a.terms, terms...)
	return a
}

func (a *aggregator[M, R]) Where(filters ...types.Filter) types.Aggregator[M, R] {
	a.filters = append(a.filters, filters...)
	return a
}

func (a *aggregator[M, R]) Having(conditions ...types.Having) types.Aggregator[M, R] {
	a.havings = append(a.havings, conditions...)
	return a
}

func (a *aggregator[M, R]) OrderBy(orders ...types.AggregateOrder) types.Aggregator[M, R] {
	a.orders = append(a.orders, orders...)
	return a
}

func (a *aggregator[M, R]) Limit(n int) types.Aggregator[M, R] {
	a.limit, a.hasLimit = n, true
	return a
}

func (a *aggregator[M, R]) Offset(n int) types.Aggregator[M, R] {
	a.offset = n
	return a
}

func (a *aggregator[M, R]) WithTable(name string) types.Aggregator[M, R] {
	a.db.WithTable(name)
	return a
}

func (a *aggregator[M, R]) WithDebug() types.Aggregator[M, R] {
	a.db.WithDebug()
	return a
}

func (a *aggregator[M, R]) WithBuildSQL(statements *[]types.SQLStatement) types.Aggregator[M, R] {
	a.db.WithBuildSQL(statements)
	return a
}

func (a *aggregator[M, R]) WithDryRun() types.Aggregator[M, R] {
	a.db.WithDryRun()
	return a
}

// Scan runs the aggregation and replaces the contents of dest.
func (a *aggregator[M, R]) Scan(dest *[]R) (err error) {
	defer a.db.reset()
	if dest == nil {
		return ErrNilDest
	}
	if err = a.db.prepare(); err != nil {
		return err
	}
	done, _, _ := a.db.trace("Aggregate")
	defer done(err)

	tx, err := a.build(true)
	if err != nil {
		return err
	}
	if a.db.dryRun {
		return a.db.collectSQL(tx.Session(&gorm.Session{DryRun: true}).Find(dest))
	}
	return tx.Scan(dest).Error
}

// ScanOne runs an ungrouped aggregation, which always produces exactly one
// row, and fills dest with it.
func (a *aggregator[M, R]) ScanOne(dest *R) (err error) {
	defer a.db.reset()
	if dest == nil {
		return ErrNilDest
	}
	if err = a.db.prepare(); err != nil {
		return err
	}
	done, _, _ := a.db.trace("AggregateOne")
	defer done(err)

	for _, t := range a.terms {
		if !t.IsMeasure() {
			return errors.Wrapf(ErrGroupedScanOne, "group key %q", t.Column)
		}
	}
	tx, err := a.build(true)
	if err != nil {
		return err
	}
	if a.db.dryRun {
		return a.db.collectSQL(tx.Session(&gorm.Session{DryRun: true}).Find(dest))
	}
	return tx.Scan(dest).Error
}

// CountGroups reports how many groups the aggregation produces. The count runs
// over the grouped query as a derived table, because COUNT(*) beside a GROUP BY
// counts the rows of each group instead of the groups themselves.
func (a *aggregator[M, R]) CountGroups(count *int) (err error) {
	defer a.db.reset()
	if count == nil {
		return ErrNilCount
	}
	if err = a.db.prepare(); err != nil {
		return err
	}
	done, _, _ := a.db.trace("AggregateCountGroups")
	defer done(err)

	// The inner query keeps its projection: the group keys decide the group
	// count, and a measure may still be referenced by HAVING. Ordering and
	// paging are dropped because neither changes how many groups exist.
	inner, err := a.build(false)
	if err != nil {
		return err
	}
	var total int64
	outer := a.db.ins.Session(&gorm.Session{NewDB: true}).Table("(?) AS grouped", inner)
	if a.db.dryRun {
		return a.db.collectSQL(outer.Session(&gorm.Session{DryRun: true}).Count(&total))
	}
	if err = outer.Count(&total).Error; err != nil {
		return err
	}
	*count = int(total)
	return nil
}

// build validates the projection and assembles the query. Ordering and paging
// are applied only when paged is set, so CountGroups can reuse the same
// assembly without them.
func (a *aggregator[M, R]) build(paged bool) (*gorm.DB, error) {
	columns, err := a.validate()
	if err != nil {
		return nil, err
	}

	table := a.db.m.GetTableName()
	if len(a.db.tableName) > 0 {
		table = a.db.tableName
	}
	// Model is what carries the schema, and the schema is what adds the
	// soft-delete condition. Table alone names the table but parses no model,
	// so an aggregate scanning into R would silently read deleted rows while a
	// List on the same model hides them.
	//
	// The model must be the allocated instance rather than a nil *M: Scan runs
	// through Rows(), which leaves Dest nil until the callback assigns Dest
	// from Model, and dereferencing a nil Model there yields an invalid value.
	tx := a.db.ins.Table(table).Model(a.db.m)

	selects := make([]string, 0, len(a.terms))
	vars := make([]any, 0)
	for _, t := range a.terms {
		sql, args := a.termExpr(t, columns)
		selects = append(selects, sql+" AS "+a.db.quoteIdent(a.alias(t)))
		vars = append(vars, args...)
	}
	tx.Statement.AddClause(clause.Select{
		Expression: clause.Expr{SQL: strings.Join(selects, ", "), Vars: vars},
	})

	if expr := a.db.renderFilters(a.filters, false, a.db.outerTableName()); expr != nil {
		tx = tx.Where(expr)
	}

	// Group keys and HAVING render the full expression rather than the output
	// alias. An alias is legal in GROUP BY and HAVING on MySQL, SQLite and
	// ClickHouse but not on PostgreSQL or SQL Server, and re-rendering costs
	// nothing, so one portable spelling replaces a per-dialect branch.
	//
	// Both go through gorm's own clause building so their values bind as
	// statement parameters. Rendering them with Dialector.Explain would be
	// wrong twice over: Explain exists to format SQL for the log, so it inlines
	// values instead of binding them, and it has no case for a nested
	// clause.Expression, so a conditional measure would reach the query as the
	// Go formatting of a struct rather than as its predicate.
	for _, t := range a.terms {
		if t.IsMeasure() {
			continue
		}
		sql, args := a.termExpr(t, columns)
		if len(args) > 0 {
			// Unreachable today: only measures carry conditions, and a group
			// key renders to a column or a bucket expression, neither of which
			// binds a value. Fail loudly rather than drop the values if a
			// future group key gains any.
			return nil, errors.Newf("group key %q renders bound values", a.alias(t))
		}
		tx = tx.Group(sql)
	}
	for _, h := range a.havings {
		sql, args := a.termExpr(h.Term, columns)
		tx = tx.Having(clause.Expr{
			SQL:  sql + " " + havingOperator(h.Op) + " ?",
			Vars: append(append([]any(nil), args...), h.Value),
		})
	}

	if paged {
		// ORDER BY may use the output alias: every supported dialect accepts
		// one there.
		for _, o := range a.orders {
			direction := types.OrderAsc
			if o.Direction == types.OrderDesc {
				direction = types.OrderDesc
			}
			tx = tx.Order(a.db.quoteIdent(a.alias(o.Term)) + " " + string(direction))
		}
		if a.hasLimit {
			tx = tx.Limit(a.limit)
		}
		if a.offset > 0 {
			tx = tx.Offset(a.offset)
		}
	}
	return tx, nil
}

// havingOperator maps a post-aggregation comparison to its SQL spelling.
func havingOperator(op types.HavingOp) string {
	switch op {
	case types.HavingOpNe:
		return "<>"
	case types.HavingOpGt:
		return ">"
	case types.HavingOpGte:
		return ">="
	case types.HavingOpLt:
		return "<"
	case types.HavingOpLte:
		return "<="
	default:
		return "="
	}
}

// alias returns the name a term is projected under, defaulting to its column.
func (a *aggregator[M, R]) alias(t types.AggregateTerm) string {
	if len(t.Alias) > 0 {
		return t.Alias
	}
	return t.Column
}

// termExpr renders one projection term, returning the SQL and the values its
// placeholders bind. A conditional measure carries its predicate as a nested
// expression, so the filter renderer stays the only place predicates are
// built.
func (a *aggregator[M, R]) termExpr(t types.AggregateTerm, columns map[string]modelschema.Column) (string, []any) {
	column := a.db.quoteIdent(t.Column)
	if !t.IsMeasure() {
		if t.Bucket == types.TimeBucketNone {
			return column, nil
		}
		return a.db.timeBucketExpr(column, t.Bucket), nil
	}

	cond := a.db.renderFilters(t.Conditions, false, a.db.outerTableName())
	switch t.Fn {
	case types.AggregateCount:
		if cond != nil {
			// COUNT(*) and COUNT(column) both become a conditional count:
			// CASE yields NULL outside the predicate, and COUNT skips NULLs.
			if len(t.Column) == 0 {
				return "COUNT(CASE WHEN ? THEN 1 END)", []any{cond}
			}
			return "COUNT(CASE WHEN ? THEN " + column + " END)", []any{cond}
		}
		if len(t.Column) == 0 {
			return "COUNT(*)", nil
		}
		return "COUNT(" + column + ")", nil
	case types.AggregateCountDistinct:
		if cond != nil {
			return "COUNT(DISTINCT CASE WHEN ? THEN " + column + " END)", []any{cond}
		}
		return "COUNT(DISTINCT " + column + ")", nil
	case types.AggregateSum:
		// An empty sum is zero, which is a fact about addition rather than a
		// guess, so SUM is always coalesced and its result field never has to
		// be a pointer. AVG, MIN and MAX are left alone on purpose: for them
		// "no rows" and "the answer happens to be zero" are different answers,
		// and collapsing them would be a silently wrong report.
		if cond != nil {
			return "COALESCE(SUM(CASE WHEN ? THEN " + column + " ELSE 0 END), 0)", []any{cond}
		}
		return "COALESCE(SUM(" + column + "), 0)", nil
	default:
		fn := string(t.Fn)
		if cond != nil {
			return fn + "(CASE WHEN ? THEN " + column + " END)", []any{cond}
		}
		return fn + "(" + column + ")", nil
	}
}

// validate checks the projection against the model schema and the result row
// before any SQL is built, and returns the model's columns for the renderer.
func (a *aggregator[M, R]) validate() (map[string]modelschema.Column, error) {
	if len(a.terms) == 0 {
		return nil, ErrEmptyProjection
	}
	columns, err := modelschema.Columns(a.db.typ)
	if err != nil {
		return nil, errors.Wrapf(err, "resolve columns of %s", a.db.typ)
	}
	byName := make(map[string]modelschema.Column, len(columns))
	for _, c := range columns {
		byName[c.DBName] = c
	}

	measures := 0
	aliases := make(map[string]struct{}, len(a.terms))
	for _, t := range a.terms {
		if t.IsMeasure() {
			measures++
		}
		if err = a.validateTerm(t, byName); err != nil {
			return nil, err
		}
		alias := a.alias(t)
		if !aliasPattern.MatchString(alias) {
			return nil, errors.Wrapf(ErrInvalidAlias, "%q", alias)
		}
		if _, dup := aliases[alias]; dup {
			return nil, errors.Wrapf(ErrDuplicateAlias, "%q", alias)
		}
		aliases[alias] = struct{}{}
	}
	// A projection of group keys alone is a plain read wearing an aggregate's
	// clothes, and List already does that better. Rejecting it keeps one
	// official path for reading rows.
	if measures == 0 {
		return nil, ErrNoAggregateFn
	}
	for _, h := range a.havings {
		if _, ok := aliases[a.alias(h.Term)]; !ok {
			return nil, errors.Wrapf(ErrAliasMissing, "HAVING references %q", a.alias(h.Term))
		}
	}
	for _, o := range a.orders {
		if _, ok := aliases[a.alias(o.Term)]; !ok {
			return nil, errors.Wrapf(ErrAliasMissing, "ORDER BY references %q", a.alias(o.Term))
		}
	}
	if err = a.validateResultRow(aliases); err != nil {
		return nil, err
	}
	return byName, nil
}

// validateTerm checks that a term names a real column and that the column's
// type accepts the function. The type check only bites on the string-name
// constructors: a term built from a generated column reference cannot reach a
// function its type rejects, because the reference does not carry the method.
func (a *aggregator[M, R]) validateTerm(t types.AggregateTerm, byName map[string]modelschema.Column) error {
	if len(t.Column) == 0 {
		// COUNT(*) is the only term without a column.
		if t.Fn == types.AggregateCount && t.IsMeasure() {
			return nil
		}
		return errors.Wrapf(ErrUnknownColumn, "term %q has no column", t.Fn)
	}
	column, ok := byName[t.Column]
	if !ok {
		return errors.Wrapf(ErrUnknownColumn, "%q", t.Column)
	}
	class := modelschema.ClassifyColumn(column.Type)
	switch {
	case t.Fn == types.AggregateSum || t.Fn == types.AggregateAvg:
		if class != modelschema.ColumnClassNumeric {
			return errors.Wrapf(ErrAggregateType, "%s over non-numeric column %q", t.Fn, t.Column)
		}
	case !t.IsMeasure() && t.Bucket != types.TimeBucketNone:
		if class != modelschema.ColumnClassTime {
			return errors.Wrapf(ErrAggregateType, "time bucket over non-time column %q", t.Column)
		}
	}
	return nil
}

// validateResultRow matches the projection aliases against the fields of R in
// both directions. gorm leaves an unmatched field at its zero value and drops
// an unmatched column, so without this check a renamed alias shows up as a
// column of zeros on a report rather than as an error.
func (a *aggregator[M, R]) validateResultRow(aliases map[string]struct{}) error {
	typ := reflect.TypeFor[R]()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return errors.Newf("aggregate result row %s is not a struct", typ)
	}
	fields, err := modelschema.Columns(typ)
	if err != nil {
		return errors.Wrapf(err, "resolve fields of result row %s", typ)
	}
	byName := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		byName[f.DBName] = struct{}{}
	}
	for alias := range aliases {
		if _, ok := byName[alias]; !ok {
			return errors.Wrapf(ErrResultFieldMissing, "%s has no field for %q", typ, alias)
		}
	}
	for _, f := range fields {
		if _, ok := aliases[f.DBName]; !ok {
			return errors.Wrapf(ErrAliasMissing, "%s.%s has no matching alias", typ, f.GoName)
		}
	}
	return nil
}
