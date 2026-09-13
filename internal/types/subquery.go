package types

import "reflect"

// Subquery is the correlated EXISTS subquery carried by FilterOpExists. It
// names the related model and the predicates narrowing its rows, at least one
// of which must be a FilterEqCol tying them to the enclosing query.
//
// A semi join is used rather than a real join on purpose: EXISTS matches a row
// at most once, so an aggregate over the outer table keeps counting each row
// once. A join to a one-to-many child multiplies the outer rows instead, and a
// SUM over that silently doubles.
type Subquery struct {
	// Model is an allocated instance of the related model. It carries the
	// child table name and its soft-delete scope, so a subquery hides the same
	// rows a List on that model hides.
	Model Model
	// Filters narrow the related rows. They must include a FilterEqCol,
	// directly or inside a group: without one the subquery would be a cross
	// join, so it fails closed instead.
	Filters []Filter
	// Negate turns the condition into NOT EXISTS.
	Negate bool
}

// FilterExists matches rows of the queried model that have at least one
// related row in C satisfying filters. EqCol predicates tie the related
// rows to the queried row, one per column pair, next to the ordinary
// conditions narrowing them:
//
//	types.FilterExists[*Item](
//	    ItemCols.SampleID.EqCol(SampleCols.ID),
//	    ItemCols.Status.Eq(StatusDone))
//	// EXISTS (SELECT 1 FROM `items`
//	//         WHERE `items`.`sample_id` = `samples`.`id`
//	//           AND `items`.`status` = ? AND `items`.`deleted_at` IS NULL)
//
// A composite key is just more pairs, rendered in the order given:
//
//	types.FilterExists[*Item](
//	    ItemCols.TenantID.EqCol(SampleCols.TenantID),
//	    ItemCols.SampleID.EqCol(SampleCols.ID),
//	    ItemCols.Status.Eq(StatusDone))
//
// The table names come from C and from the queried model; the predicate
// carries only the two column names. A subquery without any FilterEqCol fails
// closed rather than matching every row: nothing to correlate on is a
// mistake, not a request for a cross join.
//
// It is an ordinary Filter, so List, Count, Export and Select all accept it;
// it is service-only and has no URL spelling, because a client-supplied
// subquery is an unbounded read of a table the endpoint never named.
func FilterExists[C Model](filters ...Filter) Filter {
	return subqueryFilter[C](filters, false)
}

// FilterNotExists matches rows that have no related row in C satisfying
// filters. Note that it is not the negation of a filtered FilterExists over the
// same rows: a row whose related rows all fail filters matches, and so does a
// row with no related rows at all. A subquery without any FilterEqCol
// fails closed here as well: negating "match nothing" would otherwise widen
// into "match everything".
func FilterNotExists[C Model](filters ...Filter) Filter {
	return subqueryFilter[C](filters, true)
}

// subqueryFilter builds the shared value of both subquery constructors. The
// related model is allocated here rather than at render time so the database
// layer needs no type parameter of its own to reach the child table.
func subqueryFilter[C Model](filters []Filter, negate bool) Filter {
	sub := Subquery{Filters: append([]Filter(nil), filters...), Negate: negate}
	typ := reflect.TypeFor[C]()
	if typ.Kind() == reflect.Pointer {
		if m, ok := reflect.TypeAssert[C](reflect.New(typ.Elem())); ok {
			sub.Model = m
		}
	}
	return Filter{op: FilterOpExists, value: sub}
}
