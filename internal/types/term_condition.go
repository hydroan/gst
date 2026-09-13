package types

// CompareOp is a comparison applied to a projected term. Only the six
// orderings exist: the pattern and set operators of FilterOp have no meaning
// over a measure or a window function.
type CompareOp string

const (
	CompareEq  CompareOp = "eq"
	CompareNe  CompareOp = "ne"
	CompareGt  CompareOp = "gt"
	CompareGte CompareOp = "gte"
	CompareLt  CompareOp = "lt"
	CompareLte CompareOp = "lte"
)

// Valid reports whether the comparison is one this package defines. An unknown
// operator would otherwise fall through to equality and silently filter by the
// wrong comparison.
func (o CompareOp) Valid() bool {
	switch o {
	case CompareEq, CompareNe, CompareGt, CompareGte, CompareLt, CompareLte:
		return true
	default:
		return false
	}
}

// TermCondition is one condition on a projected term: a Having condition on a
// measure, or a Qualify condition on a window function. It carries the term
// itself rather than an alias string, which has two consequences: a condition
// can never name a term the projection did not declare, and the renderer can
// emit the full expression instead of the alias, which HAVING requires because
// PostgreSQL does not accept an output alias there.
type TermCondition struct {
	term  Term
	op    CompareOp
	value any
}

// Eq, Ne, Gt, Gte, Lt and Lte build a Having or Qualify condition on the
// term. The value type is checked when the query is built, because a
// function's value type follows the function rather than its column: COUNT
// always yields an integer, AVG a float, and SUM widens.
func (t Term) Eq(value any) TermCondition { return TermCondition{term: t, op: CompareEq, value: value} }

func (t Term) Ne(value any) TermCondition { return TermCondition{term: t, op: CompareNe, value: value} }

func (t Term) Gt(value any) TermCondition { return TermCondition{term: t, op: CompareGt, value: value} }

func (t Term) Gte(value any) TermCondition {
	return TermCondition{term: t, op: CompareGte, value: value}
}

func (t Term) Lt(value any) TermCondition { return TermCondition{term: t, op: CompareLt, value: value} }

func (t Term) Lte(value any) TermCondition {
	return TermCondition{term: t, op: CompareLte, value: value}
}

// TermConditionTermOf returns the term a condition constrains.
func TermConditionTermOf(c TermCondition) Term { return c.term }

// TermConditionOpOf returns the comparison a condition applies.
func TermConditionOpOf(c TermCondition) CompareOp { return c.op }

// TermConditionValueOf returns the value a condition compares the term with.
func TermConditionValueOf(c TermCondition) any { return c.value }

// NewTermCondition builds a condition from its parts, without the checks the
// comparison methods keep. It serves the framework's own tests; the public
// gst package does not forward it.
func NewTermCondition(term Term, op CompareOp, value any) TermCondition {
	return TermCondition{term: term, op: op, value: value}
}
