package database

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The select builder: the specification a Select call assembles, its
// terminals, and the shape the renderer and the validator agree on. The
// grouped side — measures, group keys, HAVING — lives in select_group.go, the
// window side in select_window.go, the constants in select_literal.go, and
// the side a union reads in union.go.

// Errors reported while a select is built. They all fail fast: a projection
// is written by service code, not parsed from a request, so a mistake in it is
// a programming error. Answering it with an empty result the way the filter
// layer answers a malformed client filter would disguise the bug as "no data
// today", which is the hardest reporting failure to trace. The grouped side
// declares its own in select_group.go, the window side in select_window.go
// and the constants in select_literal.go.
var (
	ErrEmptyProjection       = errors.New("projection is empty")
	ErrPlainSelect           = errors.New("projection declares neither an aggregate nor a window function: read rows with List, read distinct keys by adding a measure such as Count; a union member and a joining select may stay plain")
	ErrInvalidAlias          = errors.New("alias is not a valid identifier")
	ErrDuplicateAlias        = errors.New("alias is declared twice")
	ErrResultFieldMissing    = errors.New("result row has no field for alias")
	ErrAliasMissing          = errors.New("projection has no alias for result row field")
	ErrNullableResultField   = errors.New("result row field must be a pointer for a term that yields NULL")
	ErrGroupedScanOne        = errors.New("ScanOne cannot run a grouped aggregation, use Scan")
	ErrScanOneRowLevel       = errors.New("ScanOne cannot run a row-level select, use Scan")
	ErrScanOnePaged          = errors.New("ScanOne cannot use Having, Limit or Page, it always reads one row")
	ErrUnknownTermFn         = errors.New("term function is not one the framework defines")
	ErrUnknownTimeBucket     = errors.New("time bucket is not one the framework defines")
	ErrUnknownCompareOp      = errors.New("having comparison is not one the framework defines")
	ErrUnknownOrderDirection = errors.New("order direction is not one the framework defines")
	ErrHavingValue           = errors.New("having compares against a value SQL cannot order")
	ErrHavingValueType       = errors.New("having or qualify compares against a value of a kind the term cannot yield; a time term takes a time.Time, which Filter.TimeValue reads from a URL filter's boundary")
	ErrOrderTermNotSelected  = errors.New("order by references a term the projection does not declare")
	ErrSelectorUnusable      = errors.New("select could not attach to the database chain")
)

// aliasPattern is what an alias must look like. An alias reaches SQL as an
// identifier rather than a bound value, so it is restricted to a plain
// identifier instead of being quoted and hoped for.
var aliasPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// selector implements types.Selector by borrowing the Database chain for
// everything an analytical read shares with a plain one: the transaction
// carried by the context, identifier quoting, the filter renderer, tracing and
// SQL collection.
type selector[M types.Model, R any] struct {
	db  *database[M]
	err error // set when the chain could not be attached; surfaced by the terminal

	// The options live here rather than on the shared chain because reset()
	// clears the chain's copies before the terminal reads them. They hold for
	// the next terminal alone, which consumeDryRun clears them after: a
	// builder read again runs for real, as it promises.
	dryRun     bool
	statements *[]types.SQLStatement
	// describing and consuming mark the selector as being read by a query
	// that joins it, so a select joined into itself, directly or through
	// the selects it joins, is refused instead of recursing without end.
	describing bool
	consuming  bool

	terms     []types.Term
	joins     []types.JoinSource
	filters   []types.Filter
	havings   []types.TermCondition
	qualifies []types.TermCondition
	orders    []types.Ordering
	limit     int
	offset    int
	hasLimit  bool

	// The pushdown a union set on this member: the union's ordering by
	// output alias and its offset plus limit, rendered inside the member so
	// it reads only the rows the union can use. buildBranch sets them on a
	// copy, never on the caller's selector.
	branchOrders []aliasOrder
	branchLimit  int
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

func (a *selector[M, R]) Join(sources ...types.JoinSource) types.Selector[M, R] {
	a.joins = append(a.joins, sources...)
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

// Limit caps the number of result rows, read from the first: a Limit after a
// Page drops the page's skip. A non-positive limit means no limit, matching
// Database.WithLimit: the two would otherwise read the same and mean
// opposite things.
func (a *selector[M, R]) Limit(n int) types.Selector[M, R] {
	a.offset = 0
	if n <= 0 {
		a.limit, a.hasLimit = 0, false
		return a
	}
	a.limit, a.hasLimit = n, true
	return a
}

// Page keeps one page of the result rows: the limit is the page's size and
// the offset the rows of the pages before it. A page below 1 is the first,
// so a request's page passes straight through; a size below 1 pages
// nothing, as Limit caps nothing then. An OFFSET never stands without a
// LIMIT, which MySQL would refuse: the two are set together here or not at
// all.
func (a *selector[M, R]) Page(page, size int) types.Selector[M, R] {
	if size <= 0 {
		a.limit, a.offset, a.hasLimit = 0, 0, false
		return a
	}
	if page < 1 {
		page = 1
	}
	a.limit, a.offset, a.hasLimit = size, (page-1)*size, true
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
	// The options are consumed even when the select never attached: this is
	// the terminal they, and the joined selects', were set for.
	defer a.consumeDryRun()
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
	defer a.consumeDryRun()
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
		// A group key makes the read grouped and a plain column, window
		// function or joined select's term makes it row-level; either yields
		// one row per group or per row, never the single row ScanOne
		// promises.
		if t.IsGroupKey() {
			return errors.Wrapf(ErrGroupedScanOne, "group key %q", types.TermColumnOf(t))
		}
		if t.IsPlain() || t.IsWindowed() || a.readsDerived(t) {
			return errors.Wrapf(ErrScanOneRowLevel, "term %q", termAlias(t))
		}
	}
	// An ungrouped aggregation is one row by definition, so paging or filtering
	// groups can only turn that row into none. Rejecting the combination keeps
	// the "always one row" contract true instead of silently returning zeros.
	if len(a.havings) > 0 || a.hasLimit {
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
	defer a.consumeDryRun()
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
	outer := a.db.annotate(a.db.ins.Session(&gorm.Session{NewDB: true})).Table("(?) AS "+a.db.quoteIdent("grouped"), inner)
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
	// buildBranchRead renders the projection as a member of a union: the
	// SELECT list in the result row's order, without a comment of its own,
	// ordered and capped as the union pushed down.
	buildBranchRead
	// buildBranchCount is buildCountInner for a member of a union.
	buildBranchCount
)

// branch reports whether the mode renders a member of a union.
func (m buildMode) branch() bool { return m == buildBranchRead || m == buildBranchCount }

// countsRows reports whether the mode renders only what decides the row
// count.
func (m buildMode) countsRows() bool { return m == buildCountInner || m == buildBranchCount }

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
	// resultOrder is the result row's fields in declaration order, the
	// order a member of a union renders its SELECT list in.
	resultOrder []string
	// main is the queried model's table, and mainInfo what the filter
	// renderer knows about its columns.
	main     string
	mainInfo tableInfo
	// joins are the joined tables in the order declared, joined the same
	// keyed by table, and tables the queried and joined tables together as
	// the filter renderer reads them; all empty without a join.
	joins  []*joinedTable
	joined map[string]*joinedTable
	tables map[string]tableInfo
	// derived maps the alias of every term the projection reads from a
	// joined select to that select's table: the term is the select's own,
	// projected again as a column of the derived table.
	derived map[string]*joinedTable
}

// build validates the projection and assembles the query in the shape the
// mode asks for.
func (a *selector[M, R]) build(mode buildMode) (*gorm.DB, error) {
	shape, err := a.validate(mode)
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
	// also lacks the operation's comment, so it is attached again here; a
	// member of a union carries none of its own, the union's statement does,
	// and a select Qualify wraps carries it on the wrap, see qualifyWrap.
	if mode.branch() || len(a.qualifies) > 0 {
		a.db.ins = a.session()
	} else {
		a.db.ins = a.db.annotate(a.session())
	}
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
	if tx, err = a.joinClauses(tx, shape); err != nil {
		return nil, err
	}

	terms := a.terms
	switch {
	case mode.countsRows() && len(a.qualifies) == 0:
		// Without a Qualify nothing outside the keys decides the count.
		terms = shape.keys
	case mode == buildBranchRead:
		// The members of a union line up by position, so every one of them
		// spells its SELECT list in the result row's order.
		terms = a.termsInResultOrder(shape)
	}
	selects := make([]string, 0, len(terms))
	vars := make([]any, 0)
	for _, t := range terms {
		sql, args, termErr := a.termExpr(t, shape)
		if termErr != nil {
			return nil, termErr
		}
		selects = append(selects, sql+" AS "+a.db.quoteIdent(termAlias(t)))
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
	whereExpr, err := a.db.renderFilters(a.filters, false, a.whereScope(shape))
	if err != nil {
		return nil, err
	}
	if whereExpr != nil {
		tx = tx.Where(whereExpr)
	}

	if tx, err = a.groupClauses(tx, shape); err != nil {
		return nil, err
	}
	tx = a.qualifyWrap(tx, mode)

	switch mode {
	case buildRead:
		// ORDER BY may use the output alias: every supported dialect accepts
		// one there, and the alias is also what the wrapped rows are named by.
		for _, o := range a.orders {
			term, direction := a.orderedTerm(o, shape)
			tx = tx.Order(a.db.quoteIdent(termAlias(term)) + " " + string(direction))
		}
		if a.hasLimit {
			tx = tx.Limit(a.limit)
		}
		if a.offset > 0 {
			tx = tx.Offset(a.offset)
		}
	case buildBranchRead:
		// The union's ordering by output alias and its offset plus limit,
		// so the member reads only the rows the union can use: the first
		// rows of every member under the union's order are all the union
		// needs to sort its page from.
		for _, o := range a.branchOrders {
			tx = tx.Order(a.db.quoteIdent(o.alias) + " " + string(o.direction))
		}
		if a.branchLimit > 0 {
			tx = tx.Limit(a.branchLimit)
		}
	}
	return tx, nil
}

// orderedTerm resolves one ordering of the select to the projected term it
// sorts by and the direction it sorts in. validate has checked that the term
// is selected, so the lookup cannot miss here.
func (a *selector[M, R]) orderedTerm(o types.Ordering, shape projectionShape) (types.Term, types.OrderDirection) {
	switch o := o.(type) {
	case types.TermOrder:
		return types.TermOrderTermOf(o), orderDirection(types.TermOrderDirectionOf(o))
	case types.Order:
		return a.selectedColumnTerm(o.Table(), o.Column(), shape.main), orderDirection(types.OrderDirectionOf(o))
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

// consumeDryRun clears the dry-run option once a terminal has read it: the
// option names the next terminal operation alone, so a builder read again
// after a dry run executes for real. The selects joined as derived tables
// are consumed with it, whether or not the terminal reached them: their
// terminal is this one.
func (a *selector[M, R]) consumeDryRun() {
	a.dryRun = false
	a.statements = nil
	if a.consuming {
		return
	}
	a.consuming = true
	defer func() { a.consuming = false }()
	for _, source := range a.joins {
		if sj, ok := source.(types.SelectJoin); ok {
			if sub, ok := sj.Select.(nestedSelect); ok {
				sub.consumeDryRun()
			}
		}
	}
}

// termLabel names a term for a message about it, the way its author wrote
// it: the constant, the column, or the function over the column.
func termLabel(t types.Term) string {
	switch {
	case t.IsLiteral():
		return fmt.Sprintf("constant %q", types.TermLiteralOf(t))
	case types.TermFnOf(t) == types.FnNone:
		return fmt.Sprintf("column %q", types.TermColumnOf(t))
	case len(types.TermColumnOf(t)) == 0:
		return string(types.TermFnOf(t))
	default:
		return fmt.Sprintf("%s over %q", types.TermFnOf(t), types.TermColumnOf(t))
	}
}

// termAlias is the name a term is projected under: its alias, or its column
// when it has none.
func termAlias(t types.Term) string {
	if len(types.TermAliasOf(t)) > 0 {
		return types.TermAliasOf(t)
	}
	return types.TermColumnOf(t)
}

// termExpr renders one projection term, returning the SQL and the values its
// placeholders bind. A conditional measure carries its predicate as a nested
// expression, so the filter renderer stays the only place predicates are
// built; a windowed term carries its window after the function.
func (a *selector[M, R]) termExpr(t types.Term, shape projectionShape) (string, []any, error) {
	if jt, derived := a.derivedOf(t, shape); derived {
		return a.derivedExpr(jt, t), nil, nil
	}
	if t.IsLiteral() {
		return literalExpr(types.TermLiteralOf(t)), nil, nil
	}
	if !t.IsMeasure() {
		return a.keyExpr(t, shape), nil, nil
	}
	sql, args, coalesce, err := a.functionExpr(t, shape)
	if err != nil {
		return "", nil, err
	}
	if t.IsWindowed() {
		if sql, args, err = a.windowExpr(t, sql, args, shape); err != nil {
			return "", nil, err
		}
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

// columnKey names a column of the query by table and column, an empty
// table meaning the queried one.
func (a *selector[M, R]) columnKey(table, column string) string {
	if len(table) == 0 {
		table = a.db.outerTableName()
	}
	return table + "." + column
}

// session returns a statement-free handle onto the same connection. It keeps
// the context and any transaction the chain joined, and drops only the clauses
// a previous terminal left behind.
func (a *selector[M, R]) session() *gorm.DB {
	return a.db.ins.Session(&gorm.Session{NewDB: true})
}
