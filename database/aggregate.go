package database

import (
	"context"
	"database/sql"
	"fmt"
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
	ErrEmptyProjection       = errors.New("aggregate projection is empty")
	ErrNoAggregateFn         = errors.New("aggregate projection declares no aggregate function, use List for a plain read")
	ErrInvalidAlias          = errors.New("aggregate alias is not a valid identifier")
	ErrDuplicateAlias        = errors.New("aggregate alias is declared twice")
	ErrUnknownColumn         = errors.New("aggregate names a column the model does not have")
	ErrAggregateType         = errors.New("aggregate function does not accept this column type")
	ErrResultFieldMissing    = errors.New("result row has no field for aggregate alias")
	ErrAliasMissing          = errors.New("aggregate projection has no alias for result row field")
	ErrGroupedScanOne        = errors.New("ScanOne cannot run a grouped aggregation, use Scan")
	ErrUnknownAggregateFn    = errors.New("aggregate function is not one the framework defines")
	ErrUnknownTimeBucket     = errors.New("time bucket is not one the framework defines")
	ErrUnknownHavingOp       = errors.New("having comparison is not one the framework defines")
	ErrConditionOnGroupKey   = errors.New("a group key cannot carry conditions, they only restrict a measure")
	ErrBucketOnMeasure       = errors.New("a measure cannot carry a time bucket, it only truncates a group key")
	ErrHavingTermNotSelected = errors.New("having references a measure the projection does not declare")
	ErrOrderTermNotSelected  = errors.New("order by references a term the projection does not declare")
	ErrNullableResultField   = errors.New("result row field must be a pointer for an aggregate that yields NULL")
	ErrScanOnePaged          = errors.New("ScanOne cannot use Having, Limit or Offset, it always reads one row")
	ErrOffsetWithoutLimit    = errors.New("Offset needs a Limit")
	ErrAggregatorUnusable    = errors.New("aggregate could not attach to the database chain")
	ErrHavingValue           = errors.New("having compares against a value SQL cannot order")
	ErrUnknownOrderDirection = errors.New("order direction is not one the framework defines")
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
	db  *database[M]
	err error // set when the chain could not be attached; surfaced by the terminal

	// The options live here rather than on the shared chain because reset()
	// clears the chain's copies after every terminal, which would silently drop
	// them from a second read off the same builder.
	dryRun     bool
	statements *[]types.SQLStatement

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
	inner, ok := Database[M](ctx).(*database[M])
	if !ok {
		// Unreachable while Database returns the concrete chain, but swallowing
		// it would surface later as a nil dereference far from the cause.
		return &aggregator[M, R]{err: ErrAggregatorUnusable}
	}
	return &aggregator[M, R]{db: inner}
}

// AggregateOn is Aggregate on an application-held database instance. See
// DatabaseOn for the instance semantics, including the panic on nil.
func AggregateOn[M types.Model, R any](ctx context.Context, instance *gorm.DB) types.Aggregator[M, R] {
	inner, ok := DatabaseOn[M](ctx, instance).(*database[M])
	if !ok {
		// Unreachable while DatabaseOn returns the concrete chain, but
		// swallowing it would surface later as a nil dereference far from
		// the cause.
		return &aggregator[M, R]{err: ErrAggregatorUnusable}
	}
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

// Limit caps the number of result rows. A non-positive limit means no limit,
// matching Database.WithLimit: the two would otherwise read the same and mean
// opposite things.
func (a *aggregator[M, R]) Limit(n int) types.Aggregator[M, R] {
	if n <= 0 {
		a.limit, a.hasLimit = 0, false
		return a
	}
	a.limit, a.hasLimit = n, true
	return a
}

// Offset skips result rows. It needs a Limit: an OFFSET without one is a
// syntax error on MySQL, so the combination is rejected when the query is
// built rather than by the database.
func (a *aggregator[M, R]) Offset(n int) types.Aggregator[M, R] {
	if n <= 0 {
		a.offset = 0
		return a
	}
	a.offset = n
	return a
}

// The option methods never touch the chain, so they stay safe on an aggregator
// that failed to attach: the error surfaces at the terminal instead of as a nil
// dereference partway through building the query.

func (a *aggregator[M, R]) WithBuildSQL(statements *[]types.SQLStatement) types.Aggregator[M, R] {
	a.dryRun = true
	a.statements = statements
	return a
}

func (a *aggregator[M, R]) WithDryRun() types.Aggregator[M, R] {
	a.dryRun = true
	return a
}

// Scan runs the aggregation and replaces the contents of dest.
func (a *aggregator[M, R]) Scan(dest *[]R) (err error) {
	if a.err != nil {
		return a.err
	}
	defer a.db.reset()
	if dest == nil {
		return ErrNilDest
	}
	if err = a.db.prepare(); err != nil {
		return err
	}
	done, _ := a.db.trace(phaseAggregate)
	// done must read the named return, not the nil err captured at defer time.
	defer func() { done(err) }()

	tx, err := a.build(buildRead)
	if err != nil {
		return err
	}
	if a.db.dryRun {
		// Find, not Scan: Scan executes through Rows, which gorm refuses in dry
		// run mode. Both build the same statement, and dry run only needs that.
		return a.db.collectSQL(tx.Session(&gorm.Session{DryRun: true}).Find(dest))
	}
	// gorm keeps the existing elements when a Scan returns no rows, so a reused
	// destination would still hold the previous result. List documents that a
	// read replaces the destination; an aggregate read behaves the same.
	*dest = (*dest)[:0]
	return scanRowsInto(tx, dest)
}

// ScanOne runs an ungrouped aggregation, which always produces exactly one
// row, and fills dest with it.
func (a *aggregator[M, R]) ScanOne(dest *R) (err error) {
	if a.err != nil {
		return a.err
	}
	defer a.db.reset()
	if dest == nil {
		return ErrNilDest
	}
	if err = a.db.prepare(); err != nil {
		return err
	}
	done, _ := a.db.trace(phaseAggregateOne)
	defer func() { done(err) }()

	for _, t := range a.terms {
		if !t.IsMeasure() {
			return errors.Wrapf(ErrGroupedScanOne, "group key %q", t.Column)
		}
	}
	// An ungrouped aggregation is one row by definition, so paging or filtering
	// groups can only turn that row into none. Rejecting the combination keeps
	// the "always one row" contract true instead of silently returning zeros.
	if len(a.havings) > 0 || a.hasLimit || a.offset > 0 {
		return ErrScanOnePaged
	}
	tx, err := a.build(buildRead)
	if err != nil {
		return err
	}
	if a.db.dryRun {
		return a.db.collectSQL(tx.Session(&gorm.Session{DryRun: true}).Find(dest))
	}
	var zero R
	*dest = zero
	return scanRowInto(tx, dest)
}

// CountGroups reports how many groups the aggregation produces. The count runs
// over the grouped query as a derived table, because COUNT(*) beside a GROUP BY
// counts the rows of each group instead of the groups themselves.
func (a *aggregator[M, R]) CountGroups(count *int) (err error) {
	if a.err != nil {
		return a.err
	}
	defer a.db.reset()
	if count == nil {
		return ErrNilCount
	}
	if err = a.db.prepare(); err != nil {
		return err
	}
	done, _ := a.db.trace(phaseAggregateCountGroups)
	defer func() { done(err) }()

	// The inner query projects only the group keys: the outer count reads
	// nothing but how many rows the derived table answers, so a measure would
	// be computed for every group and then thrown away. HAVING still filters
	// the groups because it renders its own expression rather than a select
	// alias. Ordering and paging are dropped because neither changes how many
	// groups exist.
	inner, err := a.build(buildCountInner)
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

// buildMode selects the shape build assembles. Validation always covers the
// full specification, so a mistake surfaces on whichever terminal runs first.
type buildMode int

const (
	// buildRead renders the full projection with ordering and paging; Scan
	// and ScanOne read it.
	buildRead buildMode = iota
	// buildCountInner renders only the group keys, without ordering or
	// paging; CountGroups wraps it in a derived table and counts its rows.
	buildCountInner
)

// build validates the projection and assembles the query in the shape the
// mode asks for.
func (a *aggregator[M, R]) build(mode buildMode) (*gorm.DB, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}

	// Every build starts from a fresh statement. The chain's gorm session keeps
	// the clauses of whatever ran on it before -- reset() clears the wrapper's
	// options but says itself that it does not replace the session -- so a
	// second read off the same builder would otherwise inherit the first
	// query's WHERE, GROUP BY and LIMIT and quietly answer a different
	// question. That shape is the one the paginated-report idiom produces:
	// Scan for the page, then CountGroups for the total.
	a.db.ins = a.session()
	a.db.dryRun = a.dryRun
	a.db.buildingSQL = a.statements != nil
	a.db.sqlStatements = a.statements

	table := a.db.m.GetTableName()
	// Model is what carries the schema, and the schema is what adds the
	// soft-delete condition. Table alone names the table but parses no model,
	// so an aggregate scanning into R would silently read deleted rows while a
	// List on the same model hides them.
	//
	// The model must be the allocated instance rather than a nil *M: Scan runs
	// through Rows(), which leaves Dest nil until the callback assigns Dest
	// from Model, and dereferencing a nil Model there yields an invalid value.
	tx := a.db.ins.Table(table).Model(a.db.m)

	terms := a.terms
	if mode == buildCountInner {
		keys := make([]types.AggregateTerm, 0, len(a.terms))
		for _, t := range a.terms {
			if !t.IsMeasure() {
				keys = append(keys, t)
			}
		}
		terms = keys
	}
	selects := make([]string, 0, len(terms))
	vars := make([]any, 0)
	for _, t := range terms {
		sql, args, termErr := a.termExpr(t)
		if termErr != nil {
			return nil, termErr
		}
		selects = append(selects, sql+" AS "+a.db.quoteIdent(a.alias(t)))
		vars = append(vars, args...)
	}
	if len(selects) == 0 {
		// Reachable only in count mode with no group keys. COUNT(*) keeps the
		// derived table an aggregate query answering exactly one row -- the
		// single group the read is -- where a plain constant would answer one
		// row per matching row.
		selects = append(selects, "COUNT(*) AS "+a.db.quoteIdent("groups"))
	}
	tx.Statement.AddClause(clause.Select{
		Expression: clause.Expr{SQL: strings.Join(selects, ", "), Vars: vars},
	})

	// A predicate the renderer cannot apply narrows a client query to nothing,
	// which is the right answer for request input. Here it would turn a report
	// into a silent zero, so the reason is surfaced instead.
	whereExpr, err := a.db.renderFilters(a.filters, false, a.db.outerScope())
	if err != nil {
		return nil, err
	}
	if whereExpr != nil {
		tx = tx.Where(whereExpr)
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
		sql, args, termErr := a.termExpr(t)
		if termErr != nil {
			return nil, termErr
		}
		if len(args) > 0 {
			// Unreachable today: only measures carry conditions, and a group
			// key renders to a column or a bucket expression, neither of which
			// binds a value. Fail loudly rather than drop the values if a
			// future group key gains any.
			return nil, errors.Newf("group key %q renders bound values", a.alias(t))
		}
		// Raw keeps gorm from quoting an already quoted expression: the
		// MySQL, PostgreSQL and SQLite quoters are idempotent, but the SQL
		// Server and ClickHouse ones are not and would emit ""col"".
		tx.Statement.AddClause(clause.GroupBy{
			Columns: []clause.Column{{Name: sql, Raw: true}},
		})
	}
	for _, h := range a.havings {
		sql, args, termErr := a.termExpr(h.Term)
		if termErr != nil {
			return nil, termErr
		}
		tx = tx.Having(clause.Expr{
			SQL:  sql + " " + havingOperator(h.Op) + " ?",
			Vars: append(append([]any(nil), args...), h.Value),
		})
	}

	if mode == buildRead {
		// ORDER BY may use the output alias: every supported dialect accepts
		// one there.
		for _, o := range a.orders {
			direction := types.OrderAsc
			if o.Direction == types.OrderDesc {
				direction = types.OrderDesc
			}
			tx = tx.Order(a.db.quoteIdent(a.alias(o.Term)) + " " + string(direction))
		}
		if a.offset > 0 && !a.hasLimit {
			return nil, ErrOffsetWithoutLimit
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
		// validate rejects an operator outside the closed set before the
		// renderer runs, so this arm means the two drifted apart. Equality is
		// the least wrong spelling, and the guard above is what keeps it
		// unreachable.
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
func (a *aggregator[M, R]) termExpr(t types.AggregateTerm) (string, []any, error) {
	column := a.db.quoteIdent(t.Column)
	if !t.IsMeasure() {
		if t.Bucket == types.TimeBucketNone {
			return column, nil, nil
		}
		return a.db.timeBucketExpr(column, t.Bucket), nil, nil
	}

	cond, condErr := a.db.renderFilters(t.Conditions, false, a.db.outerScope())
	if condErr != nil {
		return "", nil, condErr
	}
	switch t.Fn {
	case types.AggregateCount:
		if cond != nil {
			// COUNT(*) and COUNT(column) both become a conditional count:
			// CASE yields NULL outside the predicate, and COUNT skips NULLs.
			if len(t.Column) == 0 {
				return "COUNT(CASE WHEN ? THEN 1 END)", []any{cond}, nil
			}
			return "COUNT(CASE WHEN ? THEN " + column + " END)", []any{cond}, nil
		}
		if len(t.Column) == 0 {
			return "COUNT(*)", nil, nil
		}
		return "COUNT(" + column + ")", nil, nil
	case types.AggregateCountDistinct:
		if cond != nil {
			return "COUNT(DISTINCT CASE WHEN ? THEN " + column + " END)", []any{cond}, nil
		}
		return "COUNT(DISTINCT " + column + ")", nil, nil
	case types.AggregateSum:
		// An empty sum is zero, which is a fact about addition rather than a
		// guess, so SUM is always coalesced and its result field never has to
		// be a pointer. AVG, MIN and MAX are left alone on purpose: for them
		// "no rows" and "the answer happens to be zero" are different answers,
		// and collapsing them would be a silently wrong report.
		if cond != nil {
			return "COALESCE(SUM(CASE WHEN ? THEN " + column + " ELSE 0 END), 0)", []any{cond}, nil
		}
		return "COALESCE(SUM(" + column + "), 0)", nil, nil
	case types.AggregateAvg, types.AggregateMin, types.AggregateMax:
		fn := string(t.Fn)
		if cond != nil {
			return fn + "(CASE WHEN ? THEN " + column + " END)", []any{cond}, nil
		}
		return fn + "(" + column + ")", nil, nil
	default:
		// validate rejects any function outside the closed set before the
		// renderer runs, so reaching this arm means the two drifted apart.
		// Composing SQL from the value would put caller text into the
		// statement, so it errors instead.
		return "", nil, errors.Wrapf(ErrUnknownAggregateFn, "%q", t.Fn)
	}
}

// validate checks the projection against the model schema and the result row
// before any SQL is built, and returns the model's columns for the renderer.
func (a *aggregator[M, R]) validate() error {
	if len(a.terms) == 0 {
		return ErrEmptyProjection
	}
	columns, err := modelschema.Columns(a.db.typ)
	if err != nil {
		return errors.Wrapf(err, "resolve columns of %s", a.db.typ)
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
			return err
		}
		alias := a.alias(t)
		if !aliasPattern.MatchString(alias) {
			return errors.Wrapf(ErrInvalidAlias, "%q", alias)
		}
		if _, dup := aliases[alias]; dup {
			return errors.Wrapf(ErrDuplicateAlias, "%q", alias)
		}
		aliases[alias] = struct{}{}
	}
	// A projection of group keys alone is a plain read wearing an aggregate's
	// clothes, and List already does that better. Rejecting it keeps one
	// official path for reading rows.
	if measures == 0 {
		return ErrNoAggregateFn
	}
	// HAVING and ORDER BY are rendered from the term they carry, not from the
	// alias, so matching the alias alone is not enough: a term with the same
	// alias but a different expression would filter or sort by something the
	// projection never declared. Requiring the whole term to match makes the
	// two agree by construction.
	for _, h := range a.havings {
		if !h.Op.Valid() {
			return errors.Wrapf(ErrUnknownHavingOp, "%q", h.Op)
		}
		if !a.isSelected(h.Term) {
			return errors.Wrapf(ErrHavingTermNotSelected, "%q", a.alias(h.Term))
		}
		// The comparison is rendered with the value bound, so anything SQL
		// cannot order either fails at the database or, for nil, compares
		// against NULL and quietly answers with no groups at all.
		if h.Value == nil {
			return errors.Wrapf(ErrHavingValue, "%q compares against nil", a.alias(h.Term))
		}
		if k := reflect.ValueOf(h.Value).Kind(); k == reflect.Slice || k == reflect.Array || k == reflect.Map {
			return errors.Wrapf(ErrHavingValue, "%q compares against a %s", a.alias(h.Term), k)
		}
		// A typed nil pointer slips past the untyped nil check above but binds
		// the same way: the driver dereferences non-nil pointers and turns a
		// nil one at any depth into NULL, which quietly answers with no groups.
		for v := reflect.ValueOf(h.Value); v.Kind() == reflect.Pointer; v = v.Elem() {
			if v.IsNil() {
				return errors.Wrapf(ErrHavingValue, "%q compares against a nil %s", a.alias(h.Term), v.Type())
			}
		}
	}
	for _, o := range a.orders {
		if !o.Direction.Valid() {
			return errors.Wrapf(ErrUnknownOrderDirection, "%q", o.Direction)
		}
		if !a.isSelected(o.Term) {
			return errors.Wrapf(ErrOrderTermNotSelected, "%q", a.alias(o.Term))
		}
	}
	if err = a.validateResultRow(aliases, byName); err != nil {
		return err
	}
	return nil
}

// validateTerm checks that a term names a real column and that the column's
// type accepts the function. The type check only bites on the string-name
// constructors: a term built from a generated column reference cannot reach a
// function its type rejects, because the reference does not carry the method.
func (a *aggregator[M, R]) validateTerm(t types.AggregateTerm, byName map[string]modelschema.Column) error {
	// The renderer composes SQL from these constants, so a value from outside
	// the closed set would reach the statement as text.
	if !t.Fn.Valid() {
		return errors.Wrapf(ErrUnknownAggregateFn, "%q", t.Fn)
	}
	if !t.Bucket.Valid() {
		return errors.Wrapf(ErrUnknownTimeBucket, "%q", t.Bucket)
	}
	// A condition on a group key and a bucket on a measure are both meaningless
	// and were previously dropped without a word, which is how a report ends up
	// silently counting the wrong rows.
	if !t.IsMeasure() && len(t.Conditions) > 0 {
		return errors.Wrapf(ErrConditionOnGroupKey, "%q", a.alias(t))
	}
	if t.IsMeasure() && t.Bucket != types.TimeBucketNone {
		return errors.Wrapf(ErrBucketOnMeasure, "%q", a.alias(t))
	}
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
		// The generated reference already blocks this at compile time for the
		// types it can classify, so the check only bites on the string-name
		// constructors. It asks ClassifyColumn rather than keeping a rule of
		// its own: a second rule admitted every struct storing itself through
		// driver.Valuer, which is also how uuid, JSON and text-backed null
		// wrappers travel, and gorm.DeletedAt is on every model. Two rules
		// disagreeing about the same type is worse than one rule being strict.
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
func (a *aggregator[M, R]) validateResultRow(aliases map[string]struct{}, sources map[string]modelschema.Column) error {
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
	// gorm leaves a non-pointer field at its zero value when it scans NULL,
	// which makes "no data" and "the answer is zero" the same number on a
	// report. Every field a NULL can actually reach must therefore be able to
	// hold it, and the requirement is enforced here rather than left to a doc
	// comment nobody reads at the call site. nullableAliases narrows the
	// demand to the measures where a NULL is reachable.
	nullable := a.nullableAliases(sources)
	for _, f := range fields {
		if _, ok := aliases[f.DBName]; !ok {
			return errors.Wrapf(ErrAliasMissing, "%s.%s has no matching alias", typ, f.GoName)
		}
		if why, isNullable := nullable[f.DBName]; isNullable && !holdsNull(f.Type) {
			return errors.Wrapf(ErrNullableResultField,
				"%s.%s holds %s; declare it as *%s or a sql.Null type",
				typ, f.GoName, why, f.Type)
		}
	}
	return nil
}

// isSelected reports whether the projection declares this exact term. Equality
// covers the whole term rather than its alias, because HAVING and ORDER BY are
// rendered from the term itself: an alias match alone would let a condition
// filter by an expression the projection never selected.
func (a *aggregator[M, R]) isSelected(t types.AggregateTerm) bool {
	for _, selected := range a.terms {
		if a.alias(selected) == a.alias(t) && reflect.DeepEqual(selected, t) {
			return true
		}
	}
	return false
}

// nullableAliases returns the aliases whose aggregate can come back NULL,
// each keyed to a clause naming the way the NULL arrives, ready for the
// validation message. SUM is absent because the renderer coalesces it to
// zero, which is the identity of addition rather than a guess.
//
// AVG, MIN and MAX return NULL when they see no value at all. Under GROUP BY
// every group holds at least one row, so a measure only meets that fate
// through one of three doors: the projection has no group keys, and the whole
// read is a single group that is empty when the filters match no rows; the
// measure carries conditions, and no row of a group passes them; or the
// source column is nullable, and a group holds only NULLs. A grouped,
// unconditional measure over a non-nullable column can keep a plain result
// field — the alternative would demand a pointer nothing ever sets to nil.
func (a *aggregator[M, R]) nullableAliases(sources map[string]modelschema.Column) map[string]string {
	grouped := false
	for _, t := range a.terms {
		if !t.IsMeasure() {
			grouped = true
			break
		}
	}
	nullable := make(map[string]string)
	for _, t := range a.terms {
		switch t.Fn {
		case types.AggregateAvg, types.AggregateMin, types.AggregateMax:
		default:
			continue
		}
		// The unknown-column case cannot be reached — validateTerm has already
		// rejected the term — but if it ever is, requiring the pointer is the
		// safe side of the guess.
		source, known := sources[t.Column]
		switch {
		case !grouped:
			nullable[a.alias(t)] = fmt.Sprintf("%s, which is NULL when the filters match no rows", t.Fn)
		case len(t.Conditions) > 0:
			nullable[a.alias(t)] = fmt.Sprintf("a conditional %s, which is NULL for a group where no row passes its conditions", t.Fn)
		case !known || holdsNull(source.Type):
			nullable[a.alias(t)] = fmt.Sprintf("%s over nullable column %q, which is NULL for a group holding only NULLs", t.Fn, t.Column)
		}
	}
	return nullable
}

// session returns a statement-free handle onto the same connection. It keeps
// the context and any transaction the chain joined, and drops only the clauses
// a previous terminal left behind.
func (a *aggregator[M, R]) session() *gorm.DB {
	return a.db.ins.Session(&gorm.Session{NewDB: true})
}

// scannerType is the interface a field implements to decode a raw database
// value itself, which is how the sql.Null wrappers represent absence.
var scannerType = reflect.TypeFor[sql.Scanner]()

// holdsNull reports whether a result row field can tell NULL apart from the
// zero value. A pointer does it by being nil; a sql.Null wrapper does it by
// carrying a Valid flag. Anything else silently reads NULL as zero, which is
// what makes "no rows" and "the answer is zero" the same number on a report.
func holdsNull(typ reflect.Type) bool {
	if typ.Kind() == reflect.Pointer {
		return true
	}
	return typ.Implements(scannerType) || reflect.PointerTo(typ).Implements(scannerType)
}
