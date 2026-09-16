package database

import (
	"context"
	"reflect"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/types"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// This file holds the model-agnostic faces an operation reading several
// models works through: the side of a selector another query reads
// (nestedSelect) and the chain such a query borrows to run its statements
// (operationChain), each with the implementations behind it. Unions and
// joined selects both go through them.

// nestedSelect is the side of a selector another query reads: a union its
// branches, a join its joined selects. *selector implements it for every
// model type, which is what lets those queries take selects over different
// models: the assertion to this interface names no M or R.
type nestedSelect interface {
	// attachError is the error the select's entry point recorded, if any.
	attachError() error
	// baseHandle is the connection handle the select was opened on.
	baseHandle() *gorm.DB
	// chainFor mints a chain of the select's model on the outer query's
	// context and handle, the chain that query runs its own statements
	// through.
	chainFor(ctx context.Context, base *gorm.DB) operationChain
	// isSelected reports whether the select projects exactly this term.
	isSelected(t types.Term) bool
	// projectsAs reports whether the select projects this term's base, the
	// term without its alias, window and conditions, and under which alias:
	// the mistake a query reading the select makes when it re-aliases,
	// windows or conditions the term instead of passing it as it is.
	projectsAs(t types.Term) (string, bool)
	// buildBranch renders the select as a member: read, ordered and capped
	// as pushed down, or reduced to what decides its row count.
	buildBranch(mode buildMode, orders []aliasOrder, limit int) (*gorm.DB, error)
	// consumeDryRun clears a dry run set on the select, the enclosing
	// query's terminal being the select's: it runs for real, whether or
	// not it reached the select, so the select read again on its own
	// executes.
	consumeDryRun()
	// describe validates the select as a joined one and reports what a query
	// joining it reads.
	describe() (derivedInfo, error)
}

// operationChain is the model-agnostic face of a database chain: what an
// operation reading several models borrows from one of them to run, trace,
// annotate and collect its statements. *database implements it.
type operationChain interface {
	prepare() error
	reset()
	useDryRun(dryRun bool, collector *[]types.SQLStatement)
	traceAs(modelName string, phase consts.Phase, batch ...int) (func(error), trace.Span)
	freshSession() *gorm.DB
	annotate(tx *gorm.DB) *gorm.DB
	quoteIdent(name string) string
	collectSQL(tx *gorm.DB) error
}

// useDryRun sets the chain's dry-run mode and statement collector for the
// operation about to run, the way a selector's build does on its own chain.
func (db *database[M]) useDryRun(dryRun bool, collector *[]types.SQLStatement) {
	db.dryRun = dryRun
	db.sqlStatements = collector
}

// freshSession returns a statement-free handle onto the chain's connection,
// keeping its context and the transaction it joined.
func (db *database[M]) freshSession() *gorm.DB {
	return db.ins.Session(&gorm.Session{NewDB: true})
}

// The methods below are the side of a selector another query reads; see
// nestedSelect. They exist on every instantiation, which is what lets a union
// or a join take selects over different models.

func (a *selector[M, R]) attachError() error { return a.err }

func (a *selector[M, R]) baseHandle() *gorm.DB { return a.db.base }

func (a *selector[M, R]) chainFor(ctx context.Context, base *gorm.DB) operationChain {
	chain, ok := databaseFor[M](ctx, base).(*database[M])
	if !ok {
		return nil
	}
	return chain
}

func (a *selector[M, R]) projectsAs(t types.Term) (string, bool) {
	t = types.TermBase(t)
	for _, selected := range a.terms {
		if reflect.DeepEqual(types.TermBase(selected), t) {
			return termAlias(selected), true
		}
	}
	return "", false
}

// buildBranch renders the selector as a member of a union, ordered and
// capped as the union pushed down. The chain is prepared here because no
// terminal of the selector runs: the union's terminal does. The pushdown is
// set on a copy so the caller's selector stays the specification it wrote.
func (a *selector[M, R]) buildBranch(mode buildMode, orders []aliasOrder, limit int) (*gorm.DB, error) {
	if err := a.db.prepare(); err != nil {
		return nil, err
	}
	member := *a
	member.branchOrders, member.branchLimit = orders, limit
	return member.build(mode)
}

// termsInResultOrder returns the projection's terms in the order of the
// result row's fields. validate has matched the aliases against the fields
// in both directions, so every field has exactly one term here.
func (a *selector[M, R]) termsInResultOrder(shape projectionShape) []types.Term {
	byAlias := make(map[string]types.Term, len(a.terms))
	for _, t := range a.terms {
		byAlias[termAlias(t)] = t
	}
	ordered := make([]types.Term, 0, len(shape.resultOrder))
	for _, alias := range shape.resultOrder {
		ordered = append(ordered, byAlias[alias])
	}
	return ordered
}

// describe validates the selector as a joined select and reports what a
// query joining it reads: its model's table, its group keys, the aliases it
// projects and the ones that can come back NULL. The chain is prepared here
// because no terminal of the selector runs.
func (a *selector[M, R]) describe() (derivedInfo, error) {
	// A select joined into itself, directly or through the selects it
	// joins, would describe itself without end.
	if a.describing {
		return derivedInfo{}, errors.Wrap(ErrJoinDuplicateTable, "a select joins itself, directly or through the selects it joins")
	}
	a.describing = true
	defer func() { a.describing = false }()
	if err := a.db.prepare(); err != nil {
		return derivedInfo{}, err
	}
	shape, err := a.validate(buildBranchRead)
	if err != nil {
		return derivedInfo{}, err
	}
	// The keys a query ties the select on are the select's own: a term it
	// reads from a select it joins itself is a column of that derived table,
	// constant within the groups its own keys define, and no column of the
	// query equals it.
	keys := make([]types.Term, 0, len(shape.keys))
	for _, key := range shape.keys {
		if _, derived := a.derivedOf(key, shape); derived {
			continue
		}
		keys = append(keys, key)
	}
	info := derivedInfo{
		table:    shape.main,
		grouped:  shape.grouped,
		keys:     keys,
		columns:  shape.columns,
		aliases:  make(map[string]struct{}, len(a.terms)),
		nullable: a.nullableAliases(shape),
	}
	for _, t := range a.terms {
		info.aliases[termAlias(t)] = struct{}{}
	}
	return info, nil
}
