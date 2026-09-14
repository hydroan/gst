package types

// TermOrder is one ORDER BY term of a select or of a window. Unlike Order it
// sorts by a projection term, which is what a TopN report ranks by.
type TermOrder struct {
	term      Term
	direction OrderDirection
}

// Asc and Desc sort the rows by this term. In the select's ORDER BY the term
// renders as its alias, which every supported dialect accepts there; inside a
// window it renders as its full expression, the only form OVER accepts.
func (t Term) Asc() TermOrder {
	return TermOrder{term: t, direction: OrderAsc}
}

func (t Term) Desc() TermOrder {
	return TermOrder{term: t, direction: OrderDesc}
}

// TermOrderTermOf returns the term an order sorts by.
func TermOrderTermOf(o TermOrder) Term { return o.term }

// TermOrderDirectionOf returns the direction an order on a term sorts in.
func TermOrderDirectionOf(o TermOrder) OrderDirection { return o.direction }

// NewTermOrder builds an order on a term from its parts, without the checks
// Asc and Desc keep. It serves the framework's own tests; the public gst
// package does not forward it.
func NewTermOrder(term Term, direction OrderDirection) TermOrder {
	return TermOrder{term: term, direction: direction}
}

func (TermOrder) sealedOrdering() {}
