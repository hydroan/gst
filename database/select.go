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

// Errors reported while a select is built. They all fail fast: a projection
// is written by service code, not parsed from a request, so a mistake in it is
// a programming error. Answering it with an empty result the way the filter
// layer answers a malformed client filter would disguise the bug as "no data
// today", which is the hardest reporting failure to trace.
var (
	ErrEmptyProjection            = errors.New("aggregate projection is empty")
	ErrPlainSelect                = errors.New("projection declares neither an aggregate nor a window function, use List for a plain read")
	ErrPlainColumnInGroupedSelect = errors.New("a column next to an aggregate must be a group key or be aggregated")
	ErrInvalidAlias               = errors.New("aggregate alias is not a valid identifier")
	ErrDuplicateAlias             = errors.New("aggregate alias is declared twice")
	ErrAggregateType              = errors.New("aggregate function does not accept this column type")
	ErrResultFieldMissing         = errors.New("result row has no field for aggregate alias")
	ErrAliasMissing               = errors.New("aggregate projection has no alias for result row field")
	ErrGroupedScanOne             = errors.New("ScanOne cannot run a grouped aggregation, use Scan")
	ErrScanOneRowLevel            = errors.New("ScanOne cannot run a row-level select, use Scan")
	ErrUnknownAggregateFn         = errors.New("aggregate function is not one the framework defines")
	ErrUnknownTimeBucket          = errors.New("time bucket is not one the framework defines")
	ErrUnknownCompareOp           = errors.New("having comparison is not one the framework defines")
	ErrConditionOnGroupKey        = errors.New("a group key cannot carry conditions, they only restrict a measure")
	ErrBucketOnMeasure            = errors.New("a measure cannot carry a time bucket, it only truncates a group key")
	ErrHavingTermNotSelected      = errors.New("having references a measure the projection does not declare")
	ErrHavingWithoutGroups        = errors.New("having needs an aggregate to restrict, a row-level select has none")
	ErrOrderTermNotSelected       = errors.New("order by references a term the projection does not declare")
	ErrNullableResultField        = errors.New("result row field must be a pointer for an aggregate that yields NULL")
	ErrScanOnePaged               = errors.New("ScanOne cannot use Having, Limit or Offset, it always reads one row")
	ErrOffsetWithoutLimit         = errors.New("Offset needs a Limit")
	ErrSelectorUnusable           = errors.New("aggregate could not attach to the database chain")
	ErrHavingValue                = errors.New("having compares against a value SQL cannot order")
	ErrUnknownOrderDirection      = errors.New("order direction is not one the framework defines")
	ErrWindowFnWithoutWindow      = errors.New("window function needs a window, declare one with Over")
	ErrWindowWithoutOrder         = errors.New("window function needs an ordered window, there is no first row without an order")
	ErrWindowOnKey                = errors.New("a group key or time bucket cannot be windowed, only a function can")
	ErrWindowCountDistinct        = errors.New("COUNT DISTINCT cannot be windowed on any supported dialect")
	ErrWindowOverGroups           = errors.New("AVG, LAG and LEAD cannot be windowed over a grouped projection, only SUM, COUNT, MIN, MAX and the ranking functions can")
	ErrWindowTermNotSelected      = errors.New("window references a key or term the projection does not declare")
	ErrWindowNested               = errors.New("a window cannot be ordered by another window function")
	ErrQualifyTermNotWindow       = errors.New("qualify references a term that is not a window function of the projection")
)

// aliasPattern is what an alias must look like. An alias reaches SQL as an
// identifier rather than a bound value, so it is restricted to a plain
// identifier instead of being quoted and hoped for.
var aliasPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// qualifiedAlias is the alias of the derived table a Qualify wraps the
// projection in; the outer conditions read the window columns through it.
const qualifiedAlias = "q"

// selector implements types.Selector by borrowing the Database chain for
// everything an analytical read shares with a plain one: the transaction
// carried by the context, identifier quoting, the filter renderer, tracing and
// SQL collection.
type selector[M types.Model, R any] struct {
	db  *database[M]
	err error // set when the chain could not be attached; surfaced by the terminal

	// The options live here rather than on the shared chain because reset()
	// clears the chain's copies after every terminal, which would silently drop
	// them from a second read off the same builder.
	dryRun     bool
	statements *[]types.SQLStatement

	terms     []types.Term
	filters   []types.Filter
	havings   []types.TermCondition
	qualifies []types.TermCondition
	orders    []types.Ordering
	limit     int
	offset    int
	hasLimit  bool
}

// Select creates an analytical read over the table of M that projects exprs
// and scans the result rows into R. The projection is declared here, at the
// entry, so a selection has exactly one place that says what it reads; see
// types.Selector for the contract and an example.
func Select[M types.Model, R any](ctx context.Context, exprs ...types.Expr) types.Selector[M, R] {
	inner, ok := Database[M](ctx).(*database[M])
	if !ok {
		// Unreachable while Database returns the concrete chain, but swallowing
		// it would surface later as a nil dereference far from the cause.
		return &selector[M, R]{err: ErrSelectorUnusable}
	}
	return &selector[M, R]{db: inner, terms: termsOf(exprs)}
}

// SelectOn is Select on an application-held database instance. See
// DatabaseOn for the instance semantics, including the panic on nil.
func SelectOn[M types.Model, R any](ctx context.Context, instance *gorm.DB, exprs ...types.Expr) types.Selector[M, R] {
	inner, ok := DatabaseOn[M](ctx, instance).(*database[M])
	if !ok {
		// Unreachable while DatabaseOn returns the concrete chain, but
		// swallowing it would surface later as a nil dereference far from
		// the cause.
		return &selector[M, R]{err: ErrSelectorUnusable}
	}
	return &selector[M, R]{db: inner, terms: termsOf(exprs)}
}

// termsOf turns the projection expressions into terms: a column reference
// selects as the plain projection of that column.
func termsOf(exprs []types.Expr) []types.Term {
	terms := make([]types.Term, 0, len(exprs))
	for _, expr := range exprs {
		terms = append(terms, types.TermOf(expr))
	}
	return terms
}

func (a *selector[M, R]) Where(filters ...types.Filter) types.Selector[M, R] {
	a.filters = append(a.filters, filters...)
	return a
}

func (a *selector[M, R]) Having(conditions ...types.TermCondition) types.Selector[M, R] {
	a.havings = append(a.havings, conditions...)
	return a
}

func (a *selector[M, R]) Qualify(conditions ...types.TermCondition) types.Selector[M, R] {
	a.qualifies = append(a.qualifies, conditions...)
	return a
}

func (a *selector[M, R]) OrderBy(orders ...types.Ordering) types.Selector[M, R] {
	a.orders = append(a.orders, orders...)
	return a
}

// Limit caps the number of result rows. A non-positive limit means no limit,
// matching Database.WithLimit: the two would otherwise read the same and mean
// opposite things.
func (a *selector[M, R]) Limit(n int) types.Selector[M, R] {
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
func (a *selector[M, R]) Offset(n int) types.Selector[M, R] {
	if n <= 0 {
		a.offset = 0
		return a
	}
	a.offset = n
	return a
}

// The option methods never touch the chain, so they stay safe on a selector
// that failed to attach: the error surfaces at the terminal instead of as a nil
// dereference partway through building the query.

func (a *selector[M, R]) WithDryRun(collector ...*[]types.SQLStatement) types.Selector[M, R] {
	a.dryRun = true
	if len(collector) > 0 {
		if collector[0] == nil {
			if a.err == nil {
				a.err = ErrNilSQLBuilder
			}
			return a
		}
		a.statements = collector[0]
	}
	return a
}

// Scan runs the select and replaces the contents of dest.
func (a *selector[M, R]) Scan(dest *[]R) (err error) {
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
	done, _ := a.db.trace(phaseSelect)
	// done must read the named return, not the nil err captured at defer time.
	defer func() { done(err) }()

	tx, err := a.build(buildRead)
	if err != nil {
		return err
	}
	if a.db.dryRun {
		// Find, not Scan: Scan executes through Rows, which gorm refuses in dry
		// run mode. Both build the same statement, and dry run only needs that.
		return a.db.collectSQL(dryRunSession(tx).Find(dest))
	}
	// gorm keeps the existing elements when a Scan returns no rows, so a reused
	// destination would still hold the previous result. List documents that a
	// read replaces the destination; an aggregate read behaves the same.
	*dest = (*dest)[:0]
	return scanRowsInto(tx, dest)
}

// ScanOne runs an ungrouped aggregation, which always produces exactly one
// row, and fills dest with it.
func (a *selector[M, R]) ScanOne(dest *R) (err error) {
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
	done, _ := a.db.trace(phaseSelectOne)
	defer func() { done(err) }()

	for _, t := range a.terms {
		// A group key makes the read grouped and a plain column or window
		// function makes it row-level; either yields one row per group or
		// per row, never the single row ScanOne promises.
		if t.IsGroupKey() {
			return errors.Wrapf(ErrGroupedScanOne, "group key %q", t.Column)
		}
		if t.IsPlain() || t.IsWindowed() {
			return errors.Wrapf(ErrScanOneRowLevel, "term %q", a.alias(t))
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
		return a.db.collectSQL(dryRunSession(tx).Find(dest))
	}
	var zero R
	*dest = zero
	return scanRowInto(tx, dest)
}

// Count reports how many rows the select produces: the groups of a grouped
// projection, the rows of a row-level one, after Having and Qualify. The
// count runs over the select as a derived table, because COUNT(*) beside a
// GROUP BY counts the rows of each group instead of the groups themselves.
// OrderBy, Limit and Offset set on the selector are ignored here: none of them
// changes how many rows exist, so pagination prepared for Scan can never skew
// the count.
func (a *selector[M, R]) Count(count *int) (err error) {
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
	done, _ := a.db.trace(phaseSelectCount)
	defer func() { done(err) }()

	// The inner query projects only what decides the row count: the group
	// keys of a grouped projection, a constant for a row-level one, and the
	// full projection only when Qualify has to read the window columns. A
	// measure would otherwise be computed for every group and then thrown
	// away. HAVING still filters the groups because it renders its own
	// expression rather than a select alias. Ordering and paging are dropped
	// because neither changes how many rows exist.
	inner, err := a.build(buildCountInner)
	if err != nil {
		return err
	}
	var total int64
	// The outer count is its own statement and carries the comment after its
	// own verb, where a database-side view reads it; the inner query keeps
	// its copy inside the derived table.
	outer := a.db.annotate(a.db.ins.Session(&gorm.Session{NewDB: true})).Table("(?) AS grouped", inner)
	if a.db.dryRun {
		return a.db.collectSQL(dryRunSession(outer).Count(&total))
	}
	if err = outer.Count(&total).Error; err != nil {
		// First-hand exit of a stack-less GORM/driver error; see the
		// error-stack contract in doc.go.
		return errors.WithStack(err)
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
	// buildCountInner renders only what decides the row count, without
	// ordering or paging; Count wraps it in a derived table and counts its
	// rows.
	buildCountInner
)

// projectionShape is what validate learned about the projection and what the
// renderer needs to know about it: whether the read is grouped, which terms
// are its group keys, and the columns of the queried model.
type projectionShape struct {
	// grouped reports a projection carrying aggregates. GROUP BY is derived
	// from keys, and a window reads the grouped rows. A row-level projection
	// carries window functions over plain columns instead.
	grouped bool
	keys    []types.Term
	columns map[string]modelschema.Column
}

// build validates the projection and assembles the query in the shape the
// mode asks for.
func (a *selector[M, R]) build(mode buildMode) (*gorm.DB, error) {
	shape, err := a.validate()
	if err != nil {
		return nil, err
	}

	// Every build starts from a fresh statement. The chain's gorm session keeps
	// the clauses of whatever ran on it before -- reset() clears the wrapper's
	// options but says itself that it does not replace the session -- so a
	// second read off the same builder would otherwise inherit the first
	// query's WHERE, GROUP BY and LIMIT and quietly answer a different
	// question. That shape is the one the paginated-report idiom produces:
	// Scan for the page, then Count for the total. The fresh statement
	// also lacks the operation's comment, so it is attached again here.
	a.db.ins = a.db.annotate(a.session())
	a.db.dryRun = a.dryRun
	a.db.sqlStatements = a.statements

	// Model is what names the table and carries the schema: gorm reads the
	// model's own TableName for the FROM clause, and the schema is what adds
	// the soft-delete condition — an aggregate scanning into R would otherwise
	// silently read deleted rows while a List on the same model hides them.
	//
	// The model must be the allocated instance rather than a nil *M: Scan runs
	// through Rows(), which leaves Dest nil until the callback assigns Dest
	// from Model, and dereferencing a nil Model there yields an invalid value.
	tx := a.db.ins.Model(a.db.m)

	terms := a.terms
	if mode == buildCountInner && len(a.qualifies) == 0 {
		// Without a Qualify nothing outside the keys decides the count.
		terms = shape.keys
	}
	selects := make([]string, 0, len(terms))
	vars := make([]any, 0)
	for _, t := range terms {
		sql, args, termErr := a.termExpr(t, shape)
		if termErr != nil {
			return nil, termErr
		}
		selects = append(selects, sql+" AS "+a.db.quoteIdent(a.alias(t)))
		vars = append(vars, args...)
	}
	if len(selects) == 0 {
		// Reachable only in count mode. An ungrouped aggregation is a single
		// group, which COUNT(*) keeps the derived table answering as exactly
		// one row; a row-level select answers one row per matching row, which
		// a constant per row keeps countable.
		if shape.grouped {
			selects = append(selects, "COUNT(*) AS "+a.db.quoteIdent("groups"))
		} else {
			selects = append(selects, "1 AS "+a.db.quoteIdent("row_marker"))
		}
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
	// alias. An output alias is accepted in GROUP BY and HAVING by MySQL, SQLite
	// and ClickHouse but rejected in HAVING by PostgreSQL, and re-rendering
	// costs nothing, so one portable spelling replaces a per-dialect branch.
	//
	// Both go through gorm's own clause building so their values bind as
	// statement parameters. Rendering them with Dialector.Explain would be
	// wrong twice over: Explain exists to format SQL for the log, so it inlines
	// values instead of binding them, and it has no case for a nested
	// clause.Expression, so a conditional measure would reach the query as the
	// Go formatting of a struct rather than as its predicate.
	for _, t := range shape.keys {
		sql, args, termErr := a.termExpr(t, shape)
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
		// MySQL, PostgreSQL and SQLite quoters are idempotent, but the
		// ClickHouse one is not and would emit ""col"".
		tx.Statement.AddClause(clause.GroupBy{
			Columns: []clause.Column{{Name: sql, Raw: true}},
		})
	}
	for _, h := range a.havings {
		sql, args, termErr := a.termExpr(h.Term, shape)
		if termErr != nil {
			return nil, termErr
		}
		tx = tx.Having(clause.Expr{
			SQL:  sql + " " + compareOperator(h.Op) + " ?",
			Vars: append(append([]any(nil), args...), h.Value),
		})
	}

	// A window function is computed after WHERE, GROUP BY and HAVING, so a
	// condition on it cannot join them. Qualify wraps the projection in a
	// derived table and filters that, the one portable spelling of the
	// QUALIFY clause some databases offer natively; ordering and paging then
	// apply to the filtered rows, outside the wrap.
	if len(a.qualifies) > 0 {
		outer := a.db.annotate(a.db.ins.Session(&gorm.Session{NewDB: true})).
			Table("(?) AS "+qualifiedAlias, tx)
		for _, q := range a.qualifies {
			outer = outer.Where(clause.Expr{
				SQL:  a.db.quoteTableColumn(qualifiedAlias, a.alias(q.Term)) + " " + compareOperator(q.Op) + " ?",
				Vars: []any{q.Value},
			})
		}
		tx = outer
	}

	if mode == buildRead {
		// ORDER BY may use the output alias: every supported dialect accepts
		// one there, and the alias is also what the wrapped rows are named by.
		for _, o := range a.orders {
			term, direction := a.orderedTerm(o)
			tx = tx.Order(a.db.quoteIdent(a.alias(term)) + " " + string(direction))
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

// orderedTerm resolves one ordering of the select to the projected term it
// sorts by and the direction it sorts in. validate has checked that the term
// is selected, so the lookup cannot miss here.
func (a *selector[M, R]) orderedTerm(o types.Ordering) (types.Term, types.OrderDirection) {
	switch o := o.(type) {
	case types.TermOrder:
		return o.Term, orderDirection(o.Direction)
	case types.Order:
		return a.selectedColumnTerm(o.Column), orderDirection(o.Direction)
	default:
		// Unreachable: Ordering is sealed to the two types above.
		return types.Term{}, types.OrderAsc
	}
}

// orderDirection normalizes a direction for rendering: the zero value reads
// as ascending, matching SQL's own default for an ORDER BY term.
func orderDirection(d types.OrderDirection) types.OrderDirection {
	if d == types.OrderDesc {
		return types.OrderDesc
	}
	return types.OrderAsc
}

// compareOperator maps a comparison of a having or qualify condition to its
// SQL spelling.
func compareOperator(op types.CompareOp) string {
	switch op {
	case types.CompareNe:
		return "<>"
	case types.CompareGt:
		return ">"
	case types.CompareGte:
		return ">="
	case types.CompareLt:
		return "<"
	case types.CompareLte:
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
func (a *selector[M, R]) alias(t types.Term) string {
	if len(t.Alias) > 0 {
		return t.Alias
	}
	return t.Column
}

// termExpr renders one projection term, returning the SQL and the values its
// placeholders bind. A conditional measure carries its predicate as a nested
// expression, so the filter renderer stays the only place predicates are
// built; a windowed term carries its window after the function.
func (a *selector[M, R]) termExpr(t types.Term, shape projectionShape) (string, []any, error) {
	if !t.IsMeasure() {
		return a.keyExpr(t), nil, nil
	}
	sql, args, coalesce, err := a.functionExpr(t)
	if err != nil {
		return "", nil, err
	}
	if t.IsWindowed() {
		if shape.grouped && isAggregateFn(t.Fn) {
			// Over a grouped projection the window reads the groups, so the
			// aggregate is applied to the group measure it names: SUM over
			// the window of the per-group sums, the count of groups, the
			// largest of the group maxima. validateWindow keeps AVG out,
			// whose nesting would answer an average of averages.
			sql = string(t.Fn) + "(" + sql + ")"
		}
		over, overArgs, overErr := a.overExpr(t, shape)
		if overErr != nil {
			return "", nil, overErr
		}
		sql += " " + over
		args = append(args, overArgs...)
	}
	if coalesce {
		// An empty sum is zero, which is a fact about addition rather than a
		// guess, so SUM is always coalesced and its result field never has to
		// be a pointer. AVG, MIN and MAX are left alone on purpose: for them
		// "no rows" and "the answer happens to be zero" are different answers,
		// and collapsing them would be a silently wrong report.
		sql = "COALESCE(" + sql + ", 0)"
	}
	return sql, args, nil
}

// keyExpr renders a group key or plain column: the column itself, or its time
// bucket.
func (a *selector[M, R]) keyExpr(t types.Term) string {
	column := a.db.quoteIdent(t.Column)
	if t.Bucket == types.TimeBucketNone {
		return column
	}
	return a.db.timeBucketExpr(column, t.Bucket)
}

// functionExpr renders the function call of a measure or window function
// without its window and without the COALESCE a SUM takes, which the caller
// adds around the complete expression; coalesce reports whether it must.
func (a *selector[M, R]) functionExpr(t types.Term) (sql string, args []any, coalesce bool, err error) {
	column := a.db.quoteIdent(t.Column)
	cond, condErr := a.db.renderFilters(t.Conditions, false, a.db.outerScope())
	if condErr != nil {
		return "", nil, false, condErr
	}
	switch t.Fn {
	case types.FnCount:
		if cond != nil {
			// COUNT(*) and COUNT(column) both become a conditional count:
			// CASE yields NULL outside the predicate, and COUNT skips NULLs.
			if len(t.Column) == 0 {
				return "COUNT(CASE WHEN ? THEN 1 END)", []any{cond}, false, nil
			}
			return "COUNT(CASE WHEN ? THEN " + column + " END)", []any{cond}, false, nil
		}
		if len(t.Column) == 0 {
			return "COUNT(*)", nil, false, nil
		}
		return "COUNT(" + column + ")", nil, false, nil
	case types.FnCountDistinct:
		if cond != nil {
			return "COUNT(DISTINCT CASE WHEN ? THEN " + column + " END)", []any{cond}, false, nil
		}
		return "COUNT(DISTINCT " + column + ")", nil, false, nil
	case types.FnSum:
		if cond != nil {
			return "SUM(CASE WHEN ? THEN " + column + " ELSE 0 END)", []any{cond}, true, nil
		}
		return "SUM(" + column + ")", nil, true, nil
	case types.FnAvg, types.FnMin, types.FnMax:
		fn := string(t.Fn)
		if cond != nil {
			return fn + "(CASE WHEN ? THEN " + column + " END)", []any{cond}, false, nil
		}
		return fn + "(" + column + ")", nil, false, nil
	case types.FnRowNumber, types.FnRank, types.FnDenseRank:
		return string(t.Fn) + "()", nil, false, nil
	case types.FnLag, types.FnLead:
		return string(t.Fn) + "(" + column + ")", nil, false, nil
	default:
		// validate rejects any function outside the closed set before the
		// renderer runs, so reaching this arm means the two drifted apart.
		// Composing SQL from the value would put caller text into the
		// statement, so it errors instead.
		return "", nil, false, errors.Wrapf(ErrUnknownAggregateFn, "%q", t.Fn)
	}
}

// validate checks the projection against the model schema and the result row
// before any SQL is built, and returns what the renderer needs to know about
// the projection's shape.
func (a *selector[M, R]) validate() (projectionShape, error) {
	shape := projectionShape{}
	if len(a.terms) == 0 {
		return shape, ErrEmptyProjection
	}
	columns, err := modelschema.Columns(a.db.typ)
	if err != nil {
		return shape, errors.Wrapf(err, "resolve columns of %s", a.db.typ)
	}
	shape.columns = make(map[string]modelschema.Column, len(columns))
	for _, c := range columns {
		shape.columns[c.DBName] = c
	}

	// The projection takes one of two shapes, and which one decides what the
	// keys mean. An aggregate or an explicit group key makes it grouped: every
	// key-shaped term is a group key, a window reads the groups, and a plain
	// column has no place, because a column next to an aggregate is either
	// grouped by or aggregated. Window functions alone make it row-level:
	// plain columns and time buckets are projected as they are.
	windowed, measures := 0, 0
	for _, t := range a.terms {
		if isWindowFn(t.Fn) && !t.IsWindowed() {
			return shape, errors.Wrapf(ErrWindowFnWithoutWindow, "%q", a.alias(t))
		}
		switch {
		case t.IsWindowed() && !t.IsMeasure():
			return shape, errors.Wrapf(ErrWindowOnKey, "%q", a.alias(t))
		case t.IsWindowed() && t.Fn == types.FnCountDistinct:
			return shape, errors.Wrapf(ErrWindowCountDistinct, "%q", a.alias(t))
		case t.IsWindowed():
			windowed++
		case t.IsMeasure():
			measures++
			shape.grouped = true
		case t.IsGroupKey() && t.Bucket == types.TimeBucketNone:
			shape.grouped = true
		}
	}
	// A projection of plain columns or keys alone is a plain read wearing a
	// select's clothes, and List already does that better. Rejecting it keeps
	// one official path for reading rows.
	if measures == 0 && windowed == 0 {
		return shape, ErrPlainSelect
	}
	for _, t := range a.terms {
		switch {
		case shape.grouped && t.IsPlain():
			return shape, errors.Wrapf(ErrPlainColumnInGroupedSelect, "%q", a.alias(t))
		case shape.grouped && t.IsGroupKey():
			shape.keys = append(shape.keys, t)
		}
	}

	aliases := make(map[string]struct{}, len(a.terms))
	for _, t := range a.terms {
		if err = a.validateTerm(t, shape); err != nil {
			return shape, err
		}
		alias := a.alias(t)
		if !aliasPattern.MatchString(alias) {
			return shape, errors.Wrapf(ErrInvalidAlias, "%q", alias)
		}
		if _, dup := aliases[alias]; dup {
			return shape, errors.Wrapf(ErrDuplicateAlias, "%q", alias)
		}
		aliases[alias] = struct{}{}
	}
	// HAVING and ORDER BY are rendered from the term they carry, not from the
	// alias, so matching the alias alone is not enough: a term with the same
	// alias but a different expression would filter or sort by something the
	// projection never declared. Requiring the whole term to match makes the
	// two agree by construction.
	if !shape.grouped && len(a.havings) > 0 {
		return shape, ErrHavingWithoutGroups
	}
	for _, h := range a.havings {
		if !a.isSelected(h.Term) {
			return shape, errors.Wrapf(ErrHavingTermNotSelected, "%q", a.alias(h.Term))
		}
		if err = validateConditionValue(h, a.alias(h.Term)); err != nil {
			return shape, err
		}
	}
	for _, q := range a.qualifies {
		if !a.isSelected(q.Term) || !q.Term.IsWindowed() {
			return shape, errors.Wrapf(ErrQualifyTermNotWindow, "%q", a.alias(q.Term))
		}
		if err = validateConditionValue(q, a.alias(q.Term)); err != nil {
			return shape, err
		}
	}
	for _, o := range a.orders {
		if err = a.validateOrdering(o); err != nil {
			return shape, err
		}
	}
	if err = a.validateResultRow(aliases, shape); err != nil {
		return shape, err
	}
	return shape, nil
}

// validateConditionValue checks the operator and value of a having or qualify
// condition. The comparison is rendered with the value bound, so anything SQL
// cannot order either fails at the database or, for nil, compares against
// NULL and quietly answers with no rows at all.
func validateConditionValue(c types.TermCondition, alias string) error {
	if !c.Op.Valid() {
		return errors.Wrapf(ErrUnknownCompareOp, "%q", c.Op)
	}
	if c.Value == nil {
		return errors.Wrapf(ErrHavingValue, "%q compares against nil", alias)
	}
	if k := reflect.ValueOf(c.Value).Kind(); k == reflect.Slice || k == reflect.Array || k == reflect.Map {
		return errors.Wrapf(ErrHavingValue, "%q compares against a %s", alias, k)
	}
	// A typed nil pointer slips past the untyped nil check above but binds
	// the same way: the driver dereferences non-nil pointers and turns a
	// nil one at any depth into NULL, which quietly answers with no rows.
	for v := reflect.ValueOf(c.Value); v.Kind() == reflect.Pointer; v = v.Elem() {
		if v.IsNil() {
			return errors.Wrapf(ErrHavingValue, "%q compares against a nil %s", alias, v.Type())
		}
	}
	return nil
}

// validateOrdering checks one ordering of the select: a term order must name a
// projected term, a column order a projected column or group key, so the
// output can never be sorted by something it does not carry.
func (a *selector[M, R]) validateOrdering(o types.Ordering) error {
	switch o := o.(type) {
	case types.TermOrder:
		if !o.Direction.Valid() {
			return errors.Wrapf(ErrUnknownOrderDirection, "%q", o.Direction)
		}
		if !a.isSelected(o.Term) {
			return errors.Wrapf(ErrOrderTermNotSelected, "%q", a.alias(o.Term))
		}
	case types.Order:
		if !o.Direction.Valid() {
			return errors.Wrapf(ErrUnknownOrderDirection, "%q", o.Direction)
		}
		if _, ok := a.selectedColumn(o.Column); !ok {
			return errors.Wrapf(ErrOrderTermNotSelected, "column %q", o.Column)
		}
	default:
		return errors.Wrapf(ErrUnknownOrderDirection, "%T", o)
	}
	return nil
}

// selectedColumn finds the projected term that carries a column as it is: a
// group key or a plain column, never a bucket, a measure or a window over it.
func (a *selector[M, R]) selectedColumn(column string) (types.Term, bool) {
	for _, t := range a.terms {
		if t.Fn == types.FnNone && t.Bucket == types.TimeBucketNone && t.Column == column {
			return t, true
		}
	}
	return types.Term{}, false
}

// selectedColumnTerm is selectedColumn for a column validation has already
// matched.
func (a *selector[M, R]) selectedColumnTerm(column string) types.Term {
	term, _ := a.selectedColumn(column)
	return term
}

// validateTerm checks that a term names a real column and that the column's
// type accepts the function. The type check only bites on minted references:
// a term built from a generated column reference cannot reach a function its
// type rejects, because the reference does not carry the method, while a
// reference minted by hand names whatever type its author chose.
func (a *selector[M, R]) validateTerm(t types.Term, shape projectionShape) error {
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
	if err := a.validateWindow(t, shape); err != nil {
		return err
	}
	// A column reference carries the table it was built for. A term naming
	// a column of another model may well name a column the queried model also
	// has, which is valid SQL over the wrong table, so the table is checked
	// before the name is. Only COUNT(*) and the ranking functions carry none.
	if len(t.Table) > 0 && t.Table != a.db.outerTableName() {
		return errors.Wrapf(ErrColumnTable, "%q belongs to table %q, the select reads %q", t.Column, t.Table, a.db.outerTableName())
	}
	if len(t.Column) == 0 {
		// COUNT(*) and the ranking functions are the terms without a column.
		if t.IsMeasure() && (t.Fn == types.FnCount || t.Fn == types.FnRowNumber || t.Fn == types.FnRank || t.Fn == types.FnDenseRank) {
			return nil
		}
		return errors.Wrapf(ErrUnknownColumn, "term %q has no column", t.Fn)
	}
	column, ok := shape.columns[t.Column]
	if !ok {
		return errors.Wrapf(ErrUnknownColumn, "%q", t.Column)
	}
	class := modelschema.ClassifyColumn(column.Type)
	switch {
	case t.Fn == types.FnSum || t.Fn == types.FnAvg:
		// The generated reference already blocks this at compile time for the
		// types it can classify, so the check only bites on minted
		// references. It asks ClassifyColumn rather than keeping a rule of
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
func (a *selector[M, R]) validateResultRow(aliases map[string]struct{}, shape projectionShape) error {
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
	// demand to the terms where a NULL is reachable.
	nullable := a.nullableAliases(shape)
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
func (a *selector[M, R]) isSelected(t types.Term) bool {
	for _, selected := range a.terms {
		if a.alias(selected) == a.alias(t) && reflect.DeepEqual(selected, t) {
			return true
		}
	}
	return false
}

// nullableAliases returns the aliases whose term can come back NULL, each
// keyed to a clause naming the way the NULL arrives, ready for the validation
// message. SUM is absent because the renderer coalesces it to zero, which is
// the identity of addition rather than a guess.
//
// AVG, MIN and MAX return NULL when they see no value at all. Under GROUP BY
// every group holds at least one row, so a measure only meets that fate
// through one of three doors: the projection has no group keys, and the whole
// read is a single group that is empty when the filters match no rows; the
// measure carries conditions, and no row of a group passes them; or the
// source column is nullable, and a group holds only NULLs. A grouped,
// unconditional measure over a non-nullable column can keep a plain result
// field — the alternative would demand a pointer nothing ever sets to nil.
//
// Over a window the partition always holds the current row, so only a
// condition or a nullable source column can leave AVG, MIN or MAX empty;
// LAG and LEAD are NULL on the first and last row of every partition; and a
// plain column is as nullable as the column it projects.
func (a *selector[M, R]) nullableAliases(shape projectionShape) map[string]string {
	grouped := len(shape.keys) > 0
	nullable := make(map[string]string)
	for _, t := range a.terms {
		source, known := shape.columns[t.Column]
		switch {
		case t.IsPlain():
			if known && holdsNull(source.Type) {
				nullable[a.alias(t)] = fmt.Sprintf("column %q, which is nullable", t.Column)
			}
			continue
		case t.Fn == types.FnLag || t.Fn == types.FnLead:
			nullable[a.alias(t)] = fmt.Sprintf("%s, which is NULL on the edge rows of a partition", t.Fn)
			continue
		case t.Fn == types.FnAvg || t.Fn == types.FnMin || t.Fn == types.FnMax:
		default:
			continue
		}
		// The unknown-column case cannot be reached — validateTerm has already
		// rejected the term — but if it ever is, requiring the pointer is the
		// safe side of the guess.
		switch {
		case !grouped && !t.IsWindowed():
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
func (a *selector[M, R]) session() *gorm.DB {
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
