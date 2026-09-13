package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// JoinSource is a source a select joins to its model. The set is closed to
// the framework: Join and LeftJoin join a model on a unique key, JoinSelect
// and LeftJoinSelect join a grouped select on its group keys.
type JoinSource = itypes.JoinSource

// Join joins model C on a unique key: JOIN, keeping only the rows of the
// query that match a row of C. The predicates are the ON condition.
func Join[C Model](on ...Filter) JoinSource {
	return itypes.Join[C](on...)
}

// LeftJoin joins model C on a unique key, keeping the rows of the query that
// match no row of C with the joined columns NULL: LEFT JOIN. The rules match
// Join; the result fields the joined columns bind to must hold NULL.
func LeftJoin[C Model](on ...Filter) JoinSource {
	return itypes.LeftJoin[C](on...)
}

// JoinSelect joins a grouped select as a derived table, JOIN (SELECT ...) AS
// jN ON ..., keeping only the rows of the query that match one of its
// groups. This is how a one-to-many relation is read beside its one side:
// the many side is grouped by the key first, so every key has one row, and
// that row is joined.
func JoinSelect[R any](sub SelectBranch[R], on ...Filter) JoinSource {
	return itypes.JoinSelect[R](sub, on...)
}

// LeftJoinSelect joins a grouped select as a derived table, keeping the rows
// of the query that match none of its groups with the select's terms NULL:
// LEFT JOIN. The rules match JoinSelect; the result fields the select's terms
// bind to must hold NULL.
func LeftJoinSelect[R any](sub SelectBranch[R], on ...Filter) JoinSource {
	return itypes.LeftJoinSelect[R](sub, on...)
}
