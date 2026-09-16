package database

import (
	"context"
	"reflect"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/types"
	"gorm.io/gorm"
)

// Errors reported while a union is built. Like the select errors they fail
// fast: a union is written by service code, and a mistake in it is a
// programming error rather than a request to answer with no rows.
var (
	ErrUnionNoBranch         = errors.New("union has no branch")
	ErrUnionBranch           = errors.New("union branch is not a select this package built")
	ErrNestedSelectOrdered   = errors.New("a select nested in a union or a join cannot carry OrderBy, Limit or Page, they belong to the outer query")
	ErrUnionBranchInstance   = errors.New("union branch was opened on another database instance")
	ErrUnionOrderNotSelected = errors.New("union order by references a column the result row does not carry")
	ErrUnionUnusable         = errors.New("union could not attach to the database chain")
)

// aliasOrder is one ordering of a union by an output column: the form the
// union renders on its own statement and pushes into every member.
type aliasOrder struct {
	alias     string
	direction types.OrderDirection
}

// union implements types.Union. It borrows a chain minted by its first branch
// for everything an operation shares — the connection and the transaction the
// context carries, tracing, the statement comment and SQL collection — and
// stacks the members the branches render.
type union[R any] struct {
	ctx   context.Context
	base  *gorm.DB
	chain operationChain
	err   error // set when the union could not be assembled; surfaced by the terminal

	branches   []nestedSelect
	orders     []types.Ordering
	limit      int
	offset     int
	hasLimit   bool
	dryRun     bool
	statements *[]types.SQLStatement
}

// UnionAll stacks the rows of branches into one result scanned into R; see
// types.Union for the contract and an example. The branches are selects
// scanning into R, over any models of the default database.
func UnionAll[R any](ctx context.Context, branches ...types.SelectBranch[R]) types.Union[R] {
	if DB() == nil {
		panic("database is not initialized")
	}
	return unionFor(ctx, DB(), branches)
}

// UnionAllOn is UnionAll on an application-held database instance, the one
// every branch was opened on with SelectOn. See DatabaseOn for the instance
// semantics, including the panic on nil.
func UnionAllOn[R any](ctx context.Context, instance *gorm.DB, branches ...types.SelectBranch[R]) types.Union[R] {
	if instance == nil {
		panic("database instance cannot be nil")
	}
	return unionFor(ctx, instance, branches)
}

// unionFor assembles the union: every branch must be a selector of this
// package that attached, opened on the same handle the union runs on. A
// branch failing any of it fails the union at its terminal rather than
// here, the way a selector that could not attach does.
func unionFor[R any](ctx context.Context, base *gorm.DB, branches []types.SelectBranch[R]) *union[R] {
	if ctx == nil {
		// Reported at the terminal like the union's other defects; the union
		// is still built, on a context of its own, so every call stays safe.
		return &union[R]{ctx: context.Background(), base: base, err: ErrNilContext}
	}
	u := &union[R]{ctx: ctx, base: base}
	if len(branches) == 0 {
		u.err = ErrUnionNoBranch
		return u
	}
	// Every branch this package built is kept even after another fails,
	// so the terminal consumes their dry-run options whatever the outcome;
	// the first failure is the union's.
	for i, b := range branches {
		member, ok := b.(nestedSelect)
		if !ok {
			if u.err == nil {
				u.err = errors.Wrapf(ErrUnionBranch, "branch %d is a %T", i, b)
			}
			continue
		}
		u.branches = append(u.branches, member)
		if u.err != nil {
			continue
		}
		if err := member.attachError(); err != nil {
			u.err = errors.Wrapf(err, "union branch %d", i)
		} else if member.baseHandle() != base {
			u.err = errors.Wrapf(ErrUnionBranchInstance, "branch %d", i)
		}
	}
	if u.err != nil {
		return u
	}
	if u.chain = u.branches[0].chainFor(ctx, base); u.chain == nil {
		// Unreachable while databaseFor returns the concrete chain, but a
		// nil chain would surface as a nil dereference far from the cause.
		u.err = ErrUnionUnusable
	}
	return u
}

func (u *union[R]) OrderBy(orders ...types.Ordering) types.Union[R] {
	u.orders = append(u.orders, orders...)
	return u
}

// Limit caps the number of stacked rows, read from the first: a Limit after a
// Page drops the page's skip. A non-positive limit means no limit, as on a
// selector.
func (u *union[R]) Limit(n int) types.Union[R] {
	u.offset = 0
	if n <= 0 {
		u.limit, u.hasLimit = 0, false
		return u
	}
	u.limit, u.hasLimit = n, true
	return u
}

// Page keeps one page of the stacked rows: the limit is the page's size and
// the offset the rows of the pages before it, as on a selector.
func (u *union[R]) Page(page, size int) types.Union[R] {
	if size <= 0 {
		u.limit, u.offset, u.hasLimit = 0, 0, false
		return u
	}
	if page < 1 {
		page = 1
	}
	u.limit, u.offset, u.hasLimit = size, (page-1)*size, true
	return u
}

func (u *union[R]) WithDryRun(collector ...*[]types.SQLStatement) types.Union[R] {
	u.dryRun = true
	if len(collector) > 0 {
		if collector[0] == nil {
			if u.err == nil {
				u.err = ErrNilSQLBuilder
			}
			return u
		}
		u.statements = collector[0]
	}
	return u
}

// consumeDryRun clears the dry-run option once a terminal has read it, on
// the union and on every branch; see selector.consumeDryRun.
func (u *union[R]) consumeDryRun() {
	u.dryRun = false
	u.statements = nil
	for _, b := range u.branches {
		b.consumeDryRun()
	}
}

// Scan runs the union and replaces the contents of dest with the stacked
// rows.
func (u *union[R]) Scan(dest *[]R) (err error) {
	// The branches' options are consumed even when the union never built:
	// this is the terminal they were set for.
	defer u.consumeDryRun()
	if u.err != nil {
		return u.err
	}
	defer u.chain.reset()
	if dest == nil {
		return ErrNilDest
	}
	if err = u.chain.prepare(); err != nil {
		return err
	}
	u.chain.useDryRun(u.dryRun, u.statements)
	done, _ := u.chain.traceAs(u.subject(), phaseUnionAll)
	// done must read the named return, not the nil err captured at defer time.
	defer func() { done(err) }()

	tx, err := u.build(buildRead)
	if err != nil {
		return err
	}
	if u.dryRun {
		// Find, not Scan: Scan executes through Rows, which gorm refuses in
		// dry run mode. Both build the same statement.
		return u.chain.collectSQL(dryRunSession(tx).Find(dest))
	}
	*dest = (*dest)[:0]
	return scanRowsInto(tx, dest)
}

// Count reports how many rows the branches produce together. It adds up the
// branches' own counts — each the count a select's Count runs, over what
// decides that branch's row count — and materializes no stacked row.
// OrderBy, Limit and Offset set on the union are ignored: none of them
// changes how many rows exist.
func (u *union[R]) Count(count *int) (err error) {
	defer u.consumeDryRun()
	if u.err != nil {
		return u.err
	}
	defer u.chain.reset()
	if count == nil {
		return ErrNilCount
	}
	if err = u.chain.prepare(); err != nil {
		return err
	}
	u.chain.useDryRun(u.dryRun, u.statements)
	done, _ := u.chain.traceAs(u.subject(), phaseUnionAllCount)
	defer func() { done(err) }()

	tx, err := u.build(buildCountInner)
	if err != nil {
		return err
	}
	var total int64
	if u.dryRun {
		return u.chain.collectSQL(dryRunSession(tx).Find(&total))
	}
	if err = tx.Scan(&total).Error; err != nil {
		// First-hand exit of a stack-less GORM/driver error; see the
		// error-stack contract in doc.go.
		return errors.WithStack(err)
	}
	*count = int(total)
	return nil
}

// build validates the union and assembles its statement in the shape the
// mode asks for: the stacked rows, or their count.
//
// Every member is a derived table, SELECT * FROM (member) AS bN, on every
// dialect. That is the one spelling of a member carrying its own ORDER BY
// and LIMIT that all four accept — SQLite allows no parentheses around a
// member and no ORDER BY inside one — and the union itself is a derived
// table too, because ClickHouse applies an ORDER BY written after the last
// member to that member alone. One spelling for four dialects beats a
// per-dialect branch that would exist only to save a wrapper the optimizers
// flatten anyway.
func (u *union[R]) build(mode buildMode) (*gorm.DB, error) {
	if err := u.validate(); err != nil {
		return nil, err
	}
	orders := u.aliasOrders()
	pushed, limit := u.pushdown(orders)

	members := make([]string, 0, len(u.branches))
	vars := make([]any, 0, len(u.branches))
	for i, b := range u.branches {
		alias := "b" + strconv.Itoa(i)
		var (
			tx  *gorm.DB
			err error
		)
		if mode == buildCountInner {
			tx, err = b.buildBranch(buildBranchCount, nil, 0)
			members = append(members, "(SELECT COUNT(*) FROM (?) AS "+u.chain.quoteIdent(alias)+")")
		} else {
			tx, err = b.buildBranch(buildBranchRead, pushed, limit)
			members = append(members, "SELECT * FROM (?) AS "+u.chain.quoteIdent(alias))
		}
		if err != nil {
			return nil, errors.Wrapf(err, "union branch %d", i)
		}
		vars = append(vars, tx)
	}

	// The union's statement is its own and carries the comment; the members
	// were rendered without one.
	session := u.chain.annotate(u.chain.freshSession())
	if mode == buildCountInner {
		// The counts are scalar subqueries added up, so no stacked row is
		// ever materialized, and the addition of integer counts stays an
		// integer on every dialect, which a SUM over them would not.
		n, counts := u.chain.quoteIdent("n"), u.chain.quoteIdent("counts")
		return session.Table("(SELECT "+strings.Join(members, " + ")+" AS "+n+") AS "+counts, vars...).Select(n), nil
	}
	tx := session.Table("("+strings.Join(members, " UNION ALL ")+") AS "+u.chain.quoteIdent("u"), vars...)
	for _, o := range orders {
		tx = tx.Order(u.chain.quoteIdent(o.alias) + " " + string(o.direction))
	}
	if u.hasLimit {
		tx = tx.Limit(u.limit)
	}
	if u.offset > 0 {
		tx = tx.Offset(u.offset)
	}
	return tx, nil
}

// validate checks the union's own clauses: every ordering names a column of
// the result row and a term ordering names a term some branch projects. The
// branches validate themselves when they are rendered.
func (u *union[R]) validate() error {
	typ, fields, err := resultRowFields[R]()
	if err != nil {
		return err
	}
	columns := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		columns[f.DBName] = struct{}{}
	}
	for _, o := range u.orders {
		ordered, err := u.orderedAlias(o)
		if err != nil {
			return err
		}
		if _, ok := columns[ordered.alias]; !ok {
			return errors.Wrapf(ErrUnionOrderNotSelected, "%s has no field for %q", typ, ordered.alias)
		}
		if to, ok := o.(types.TermOrder); ok && !u.anyBranchSelects(types.TermOrderTermOf(to)) {
			return errors.Wrapf(ErrUnionOrderNotSelected, "no branch projects the term %q", ordered.alias)
		}
	}
	return nil
}

// orderedAlias resolves one ordering of the union to the output column it
// sorts by and the direction: a term ordering by the term's alias, a column
// ordering by the column's name, which is the alias a column projects under
// unless renamed.
func (u *union[R]) orderedAlias(o types.Ordering) (aliasOrder, error) {
	switch o := o.(type) {
	case types.TermOrder:
		if !types.TermOrderDirectionOf(o).Valid() {
			return aliasOrder{}, errors.Wrapf(ErrUnknownOrderDirection, "%q", types.TermOrderDirectionOf(o))
		}
		return aliasOrder{alias: termAlias(types.TermOrderTermOf(o)), direction: orderDirection(types.TermOrderDirectionOf(o))}, nil
	case types.Order:
		if !types.OrderDirectionOf(o).Valid() {
			return aliasOrder{}, errors.Wrapf(ErrUnknownOrderDirection, "%q", types.OrderDirectionOf(o))
		}
		return aliasOrder{alias: o.Column(), direction: orderDirection(types.OrderDirectionOf(o))}, nil
	default:
		return aliasOrder{}, errors.Wrapf(ErrUnknownOrderDirection, "%T", o)
	}
}

// aliasOrders resolves every ordering of the union; validate has checked
// each, so the resolution cannot fail here.
func (u *union[R]) aliasOrders() []aliasOrder {
	orders := make([]aliasOrder, 0, len(u.orders))
	for _, o := range u.orders {
		ordered, _ := u.orderedAlias(o)
		orders = append(orders, ordered)
	}
	return orders
}

// pushdown decides what every member reads under the union's paging: with a
// limit, the union's ordering and its offset plus limit, because the first
// offset-plus-limit rows of every member under that order are all the union
// can ever place on its page, and each member finds them by its own index;
// without a limit nothing, since ordering members the union re-sorts whole
// would be wasted work.
func (u *union[R]) pushdown(orders []aliasOrder) ([]aliasOrder, int) {
	if !u.hasLimit {
		return nil, 0
	}
	return orders, u.offset + u.limit
}

// anyBranchSelects reports whether some branch projects exactly this term.
func (u *union[R]) anyBranchSelects(t types.Term) bool {
	for _, b := range u.branches {
		if b.isSelected(t) {
			return true
		}
	}
	return false
}

// subject names the union in spans and logs: the result row type, which is
// what the union reads several models into. An unnamed row type reads as
// "union".
func (u *union[R]) subject() string {
	typ := reflect.TypeFor[R]()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if name := typ.Name(); len(name) > 0 {
		return name
	}
	return "union"
}

// The methods below are the side of a selector another query reads; see
// nestedSelect. They exist on every instantiation, which is what lets a union
// or a join take selects over different models.
