package database

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/types"
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
	ErrScanOnePaged          = errors.New("ScanOne cannot use Having, Limit or Offset, it always reads one row")
	ErrUnknownTermFn         = errors.New("term function is not one the framework defines")
	ErrUnknownTimeBucket     = errors.New("time bucket is not one the framework defines")
	ErrUnknownCompareOp      = errors.New("having comparison is not one the framework defines")
	ErrUnknownOrderDirection = errors.New("order direction is not one the framework defines")
	ErrHavingValue           = errors.New("having compares against a value SQL cannot order")
	ErrHavingValueType       = errors.New("having or qualify compares against a value of a kind the term cannot yield; a time term takes a time.Time, which Filter.TimeValue reads from a URL filter's boundary")
	ErrOrderTermNotSelected  = errors.New("order by references a term the projection does not declare")
	ErrOffsetWithoutLimit    = errors.New("Offset needs a Limit")
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
			return errors.Wrapf(ErrGroupedScanOne, "group key %q", t.Column)
		}
		if t.IsPlain() || t.IsWindowed() || a.readsDerived(t) {
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
			tx = tx.Order(a.db.quoteIdent(a.alias(term)) + " " + string(direction))
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
		return o.Term, orderDirection(o.Direction)
	case types.Order:
		return a.selectedColumnTerm(o.Table, o.Column, shape.main), orderDirection(o.Direction)
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

// alias returns the name a term is projected under, defaulting to its column.
func (a *selector[M, R]) alias(t types.Term) string { return termAlias(t) }

// termLabel names a term for a message about it, the way its author wrote
// it: the constant, the column, or the function over the column.
func termLabel(t types.Term) string {
	switch {
	case t.IsLiteral():
		return fmt.Sprintf("constant %q", t.Literal)
	case t.Fn == types.FnNone:
		return fmt.Sprintf("column %q", t.Column)
	case len(t.Column) == 0:
		return string(t.Fn)
	default:
		return fmt.Sprintf("%s over %q", t.Fn, t.Column)
	}
}

// termAlias is the name a term is projected under: its alias, or its column
// when it has none.
func termAlias(t types.Term) string {
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
	if jt, derived := a.derivedOf(t, shape); derived {
		return a.derivedExpr(jt, t), nil, nil
	}
	if t.IsLiteral() {
		return literalExpr(t.Literal), nil, nil
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

// validate checks the projection against the model schema and the result row
// before any SQL is built, and returns what the renderer needs to know about
// the projection's shape. The mode says whether the projection is read on its
// own or as a member of a union, which changes two rules: a member may be a
// plain projection of columns, and it carries no ordering or paging.
func (a *selector[M, R]) validate(mode buildMode) (projectionShape, error) {
	shape := projectionShape{}
	if len(a.terms) == 0 {
		return shape, ErrEmptyProjection
	}
	if mode.branch() && (len(a.orders) > 0 || a.hasLimit || a.offset > 0) {
		return shape, ErrNestedSelectOrdered
	}
	// Checked here rather than where paging renders, so Count refuses the
	// specification Scan would: validation covers the whole of it.
	if !mode.branch() && a.offset > 0 && !a.hasLimit {
		return shape, ErrOffsetWithoutLimit
	}
	columns, err := modelschema.Columns(a.db.typ)
	if err != nil {
		return shape, errors.Wrapf(err, "resolve columns of %s", a.db.typ)
	}
	shape.columns = columnsByName(columns)
	shape.main = a.db.outerTableName()
	shape.mainInfo = tableInfoOf(columns)
	if err = a.resolveJoins(&shape); err != nil {
		return shape, err
	}
	if shape.derived, err = a.derivedTerms(shape); err != nil {
		return shape, err
	}
	// A joined select's term altered — passed under another alias, or with
	// a window or conditions of its own — is not the term the select
	// projects: it would read as a measure or a column of the select's model
	// and fail on some other term of the projection, far from the mistake,
	// so it is named here. A term the query could compute itself is the
	// query's own under an alias of its own; under the select's alias it
	// would pass for the select's term, and is refused; one a select projects
	// alike was refused above.
	for _, t := range a.terms {
		if _, derived := a.derivedOf(t, shape); derived {
			continue
		}
		own := a.ownTerm(t)
		// Every joined select is asked, whichever of them projects the term.
		for _, jt := range shape.joins {
			if jt.sub == nil {
				continue
			}
			alias, projected := jt.sub.projectsAs(t)
			switch {
			case !projected:
			case !own:
				return shape, errors.Wrapf(ErrJoinSelectColumn, "%q is the term %q of the joined select over %q altered, under another alias or with a window or conditions of its own; pass the very term the select projects", a.alias(t), alias, jt.table)
			case a.alias(t) == alias:
				return shape, errors.Wrapf(ErrDuplicateAlias, "%q is the term the joined select over %q projects, with a window or conditions of its own: neither the select's term read through nor the query's own kept apart; pass the very term to read it through, or alias the query's own term differently", alias, jt.table)
			}
		}
	}

	// The projection takes one of two shapes, and which one decides what the
	// keys mean. An aggregate or an explicit group key makes it grouped: every
	// key-shaped term is a group key, a window reads the groups, and a plain
	// column has no place, because a column next to an aggregate is either
	// grouped by or aggregated. Window functions alone make it row-level:
	// plain columns and time buckets are projected as they are.
	windowed, measures := 0, 0
	for _, t := range a.terms {
		if _, derived := a.derivedOf(t, shape); derived {
			// A joined select's term is a column of the derived table, whatever
			// function it applied inside the select: it neither groups nor
			// windows the query.
			continue
		}
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
	// one official path for reading rows. Stacked into a union it is a
	// report again, and across a join it reads what List cannot, so a member
	// and a joining select are allowed to be one.
	if measures == 0 && windowed == 0 && !mode.branch() && len(shape.joins) == 0 {
		return shape, ErrPlainSelect
	}
	for _, t := range a.terms {
		if _, derived := a.derivedOf(t, shape); derived {
			continue
		}
		switch {
		case shape.grouped && t.IsPlain():
			return shape, errors.Wrapf(ErrPlainColumnInGroupedSelect, "%q", a.alias(t))
		case shape.grouped && t.IsGroupKey():
			shape.keys = append(shape.keys, t)
		}
	}
	if err = a.groupDerivedTerms(&shape); err != nil {
		return shape, err
	}

	aliases := make(map[string]struct{}, len(a.terms))
	for _, t := range a.terms {
		if err = a.validateTerm(t, shape); err != nil {
			return shape, err
		}
		alias := a.alias(t)
		if len(alias) == 0 {
			return shape, errors.Wrapf(ErrInvalidAlias, "%s carries no alias; a term without a column is named with As", termLabel(t))
		}
		if !aliasPattern.MatchString(alias) {
			return shape, errors.Wrapf(ErrInvalidAlias, "%q on %s", alias, termLabel(t))
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
	if err = a.validateHaving(shape); err != nil {
		return shape, err
	}
	if err = a.validateQualify(shape); err != nil {
		return shape, err
	}
	for _, o := range a.orders {
		if err = a.validateOrdering(o, shape); err != nil {
			return shape, err
		}
	}
	if shape.resultOrder, err = a.validateResultRow(aliases, shape); err != nil {
		return shape, err
	}
	return shape, nil
}

// validateConditionValue checks the operator and value of a having or qualify
// condition. The comparison is rendered with the value bound, so anything SQL
// cannot order either fails at the database or, for nil, compares against
// NULL and quietly answers with no rows at all; and a value of another kind
// than the term yields — text against a count, a number against a name — is
// a comparison some dialects answer with no rows rather than an error, so it
// is refused where both kinds are known.
func validateConditionValue(c types.TermCondition, alias string, yields valueKind) error {
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
	v := reflect.ValueOf(c.Value)
	for ; v.Kind() == reflect.Pointer; v = v.Elem() {
		if v.IsNil() {
			return errors.Wrapf(ErrHavingValue, "%q compares against a nil %s", alias, v.Type())
		}
	}
	if given := valueKindOf(v); yields != kindUnknown && given != kindUnknown && given != yields {
		return errors.Wrapf(ErrHavingValueType, "%q yields %s, the value %v is %s", alias, yields, c.Value, given)
	}
	return nil
}

// valueKind is the kind of value a term yields or a condition compares
// against, as far as a comparison between the two can be judged before the
// query runs: a number, text, or an instant. Anything else is unknown and
// left to the database.
type valueKind int

const (
	kindUnknown valueKind = iota
	kindNumeric
	kindText
	kindTime
)

func (k valueKind) String() string {
	switch k {
	case kindNumeric:
		return "a number"
	case kindText:
		return "text"
	case kindTime:
		return "an instant"
	default:
		return "an unknown kind"
	}
}

// valueKindOf classifies a condition's value by its Go type, the pointer
// already dereferenced.
func valueKindOf(v reflect.Value) valueKind {
	if v.Type() == timeType {
		return kindTime
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return kindNumeric
	case reflect.String:
		return kindText
	default:
		return kindUnknown
	}
}

// termKind reports the kind of value a term yields: the counts and ranks
// are numbers whatever they count, AVG and SUM are numbers over the numeric
// columns they exist on, a time bucket and a literal are text, and every
// other term yields what its column stores. A joined select's term is
// classified by the select that projects it, which is out of reach here.
func (a *selector[M, R]) termKind(t types.Term, shape projectionShape) valueKind {
	if _, derived := a.derivedOf(t, shape); derived {
		return kindUnknown
	}
	switch {
	case t.IsLiteral(), t.Bucket != types.TimeBucketNone:
		return kindText
	case t.Fn == types.FnCount, t.Fn == types.FnCountDistinct, t.Fn == types.FnRowNumber,
		t.Fn == types.FnRank, t.Fn == types.FnDenseRank, t.Fn == types.FnSum, t.Fn == types.FnAvg:
		return kindNumeric
	}
	column, err := a.columnOf(t.Table, t.Column, shape)
	if err != nil {
		return kindUnknown
	}
	switch modelschema.ClassifyColumn(column.Type) {
	case modelschema.ColumnClassNumeric:
		return kindNumeric
	case modelschema.ColumnClassTime:
		return kindTime
	}
	typ := column.Type
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() == reflect.String {
		return kindText
	}
	return kindUnknown
}

// validateOrdering checks one ordering of the select: a term order must name a
// projected term, a column order a projected column or group key, so the
// output can never be sorted by something it does not carry.
func (a *selector[M, R]) validateOrdering(o types.Ordering, shape projectionShape) error {
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
		if _, ok := a.selectedColumn(o.Table, o.Column, shape.main); !ok {
			return errors.Wrapf(ErrOrderTermNotSelected, "column %q, which a plain name matches by column name rather than by alias; order by the term itself to sort by its alias", o.Column)
		}
	default:
		return errors.Wrapf(ErrUnknownOrderDirection, "%T", o)
	}
	return nil
}

// selectedColumn finds the projected term that carries a column as it is: a
// group key or a plain column, never a bucket, a measure or a window over it.
// The table narrows the match to that table's column, which tells two joined
// tables' columns of one name apart; an ordering built from a plain name
// carries none and names the queried model's column, as a plain name does
// everywhere else, never whichever table's column happens to be projected
// first.
func (a *selector[M, R]) selectedColumn(table, column, main string) (types.Term, bool) {
	if len(table) == 0 {
		table = main
	}
	for _, t := range a.terms {
		if t.Fn != types.FnNone || t.Bucket != types.TimeBucketNone || t.Column != column {
			continue
		}
		if termTable := t.Table; len(termTable) > 0 && termTable != table || len(termTable) == 0 && table != main {
			continue
		}
		return t, true
	}
	return types.Term{}, false
}

// selectedColumnTerm is selectedColumn for a column validation has already
// matched.
func (a *selector[M, R]) selectedColumnTerm(table, column, main string) types.Term {
	term, _ := a.selectedColumn(table, column, main)
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
		return errors.Wrapf(ErrUnknownTermFn, "%q", t.Fn)
	}
	if !t.Bucket.Valid() {
		return errors.Wrapf(ErrUnknownTimeBucket, "%q", t.Bucket)
	}
	if _, derived := a.derivedOf(t, shape); derived {
		// The joined select validated the term against its own model; here
		// it is a column of the derived table, with nothing left to check.
		return nil
	}
	if err := a.validateGrouping(t); err != nil {
		return err
	}
	if len(t.Conditions) > 0 {
		// A measure's conditions render inside its CASE, which Count leaves
		// out of its statement; they are rendered here once so a predicate
		// the renderer cannot place fails whichever terminal runs first.
		if _, err := a.db.renderFilters(t.Conditions, false, a.whereScope(shape)); err != nil {
			return errors.Wrapf(err, "%q", a.alias(t))
		}
	}
	if err := a.validateWindow(t, shape); err != nil {
		return err
	}
	if t.IsLiteral() {
		// A constant names no column and reads no schema.
		return validateLiteral(t)
	}
	if len(t.Column) == 0 {
		// COUNT(*) and the ranking functions are the terms without a column.
		if t.IsMeasure() && (t.Fn == types.FnCount || t.Fn == types.FnRowNumber || t.Fn == types.FnRank || t.Fn == types.FnDenseRank) {
			return nil
		}
		return errors.Wrapf(ErrUnknownColumn, "term %q has no column", t.Fn)
	}
	// A column reference carries the table it was built for, which columnOf
	// checks before the name: the queried model's, or a joined model's.
	column, err := a.columnOf(t.Table, t.Column, shape)
	if err != nil {
		return err
	}
	// Under GROUP BY a joined row is shared by every row of the group that
	// matched it, so only the aggregates that answer the same over repeats
	// may read a joined column.
	if _, joined := shape.joined[t.Table]; joined && shape.grouped && t.IsMeasure() && !joinedMeasureAllowed(t.Fn) {
		return errors.Wrapf(ErrJoinMeasure, "%s over %q of %q", t.Fn, t.Column, t.Table)
	}
	return a.validateColumnClass(t, column)
}

// validateResultRow matches the projection aliases against the fields of R in
// both directions, and returns the fields in declaration order. gorm leaves an
// unmatched field at its zero value and drops an unmatched column, so without
// this check a renamed alias shows up as a column of zeros on a report rather
// than as an error.
func (a *selector[M, R]) validateResultRow(aliases map[string]struct{}, shape projectionShape) ([]string, error) {
	typ, fields, err := resultRowFields[R]()
	if err != nil {
		return nil, err
	}
	order := make([]string, 0, len(fields))
	byName := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		order = append(order, f.DBName)
		byName[f.DBName] = struct{}{}
	}
	for alias := range aliases {
		if _, ok := byName[alias]; !ok {
			return nil, errors.Wrapf(ErrResultFieldMissing, "%s has no field for %q", typ, alias)
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
			return nil, errors.Wrapf(ErrAliasMissing, "%s.%s has no matching alias", typ, f.GoName)
		}
		if why, isNullable := nullable[f.DBName]; isNullable && !holdsNull(f.Type) {
			return nil, errors.Wrapf(ErrNullableResultField,
				"%s.%s holds %s; declare it as *%s or a sql.Null type",
				typ, f.GoName, why, f.Type)
		}
	}
	return order, nil
}

// resultRowFields resolves the result row type R to its struct type and its
// fields in declaration order, the order a union member's SELECT list
// follows. A pointer row type is read through.
func resultRowFields[R any]() (reflect.Type, []modelschema.Column, error) {
	typ := reflect.TypeFor[R]()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return typ, nil, errors.Newf("result row %s is not a struct", typ)
	}
	columns, err := modelschema.Columns(typ)
	if err != nil {
		return typ, nil, errors.Wrapf(err, "resolve fields of result row %s", typ)
	}
	// Columns answers in column-name order from a shared cache; the union
	// wants the row as it was declared, so a copy is put back into field
	// order, embedded structs included, by the index path.
	fields := slices.Clone(columns)
	slices.SortFunc(fields, func(a, b modelschema.Column) int { return slices.Compare(a.Index, b.Index) })
	return typ, fields, nil
}

// isSelected reports whether the projection declares this exact term. Equality
// covers the whole term rather than its alias, because HAVING and ORDER BY are
// rendered from the term itself: an alias match alone would let a condition
// filter by an expression the projection never selected. The alias is
// compared as projected, so a term spelled without one is the term under
// its default.
func (a *selector[M, R]) isSelected(t types.Term) bool {
	t.Alias = a.alias(t)
	for _, selected := range a.terms {
		selected.Alias = a.alias(selected)
		if reflect.DeepEqual(selected, t) {
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
	proven := a.notNullColumns()
	for _, t := range a.terms {
		// A joined select's term is NULL where a LEFT JOIN matched no group,
		// and where the select itself answered NULL.
		if jt, derived := a.derivedOf(t, shape); derived {
			switch why, isNullable := jt.derived.nullable[a.alias(t)]; {
			case jt.left:
				nullable[a.alias(t)] = fmt.Sprintf("term %q of the joined select, which is NULL when the LEFT JOIN matches no group", a.alias(t))
			case isNullable:
				nullable[a.alias(t)] = why
			}
			continue
		}
		// A column of a LEFT JOIN table is NULL on every row the join left
		// unmatched, whatever reads it, except the counts, which count no
		// row as zero, and SUM, which the renderer coalesces to zero.
		if jt, joined := shape.joined[t.Table]; joined && jt.left && t.Fn != types.FnCount && t.Fn != types.FnCountDistinct && t.Fn != types.FnSum {
			nullable[a.alias(t)] = fmt.Sprintf("column %q of %q, which is NULL when the LEFT JOIN matches no row", t.Column, t.Table)
			continue
		}
		source, err := a.columnOf(t.Table, t.Column, shape)
		known := err == nil
		switch {
		case t.IsPlain():
			if known && nullableColumn(source) && !proven[a.columnKey(t.Table, t.Column)] {
				nullable[a.alias(t)] = fmt.Sprintf("column %q, which is nullable unless a condition on it in Where keeps NULL out", t.Column)
			}
			continue
		case t.IsGroupKey():
			// The rows without a value form a group of their own, keyed NULL;
			// a bucket of NULL is NULL as well.
			if known && nullableColumn(source) && !proven[a.columnKey(t.Table, t.Column)] {
				nullable[a.alias(t)] = fmt.Sprintf("group key %q over a nullable column, NULL for the rows without one unless a condition on it in Where keeps them out", t.Column)
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
		case !known || (nullableColumn(source) && !proven[a.columnKey(t.Table, t.Column)]):
			nullable[a.alias(t)] = fmt.Sprintf("%s over nullable column %q, NULL for a group holding only NULLs unless a condition on it in Where keeps them out", t.Fn, t.Column)
		}
	}
	return nullable
}

// notNullColumns returns the columns the WHERE keeps NULL out of, keyed by
// columnKey. A condition on a column is never true of NULL, IS NULL apart,
// so a row whose column is NULL fails it. A condition AND-ed at the top
// level, or inside an AND group, holds of every row read; one inside an OR
// group need not, and a subquery says nothing of the row's own columns.
func (a *selector[M, R]) notNullColumns() map[string]bool {
	proven := make(map[string]bool)
	var walk func(filters []types.Filter)
	walk = func(filters []types.Filter) {
		for _, f := range filters {
			switch f.Op {
			case types.FilterOpAnd:
				if members, ok := f.Value.([]types.Filter); ok {
					walk(members)
				}
			case types.FilterOpOr, types.FilterOpExists, types.FilterOpFalse:
			case types.FilterOpIsNull:
				if isNull, ok := f.Value.(bool); ok && !isNull {
					proven[a.columnKey(f.Table, f.Column)] = true
				}
			default:
				if len(f.Column) > 0 {
					proven[a.columnKey(f.Table, f.Column)] = true
				}
			}
		}
	}
	walk(a.filters)
	return proven
}

// nullableColumn reports whether a model column can hold NULL: its Go type
// tells NULL apart and the schema does not forbid it.
func nullableColumn(c modelschema.Column) bool { return holdsNull(c.Type) && !c.NotNull }

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
