package types

import "reflect"

// JoinSource is a source a select joins to its model. The set is closed to
// this package: a ModelJoin, built by Join and LeftJoin, joins a model on a
// unique key; a SelectJoin, built by JoinSelect and LeftJoinSelect, joins a
// grouped select on its group keys.
//
// A join is allowed only where it cannot multiply rows. The ON predicates
// must pin the joined model's primary key or one of its unique indexes — or
// every group key of the joined select — with equalities, each to a column
// of the queried model or of an earlier joined one, written with
// Column.EqCol, or to a value, so that every row of the query matches at
// most one joined row; the framework proves this from the model's
// declarations when the query is built and rejects a join it cannot prove. A
// one-to-many relation is read through a joined select that groups the many
// side by the key, or with FilterExists when only the existence of related
// rows matters.
//
// The joined model's soft-delete condition is written into the ON, so a join
// never reads a row a List on that model hides. Inside a grouped projection a
// joined column may be a group key or carry MIN, MAX or COUNT DISTINCT, the
// aggregates that answer the same however many rows of a group share the
// joined row; a SUM or COUNT over it would count that row once per row of the
// group. A column of a model joined with LeftJoin can come back NULL, so its
// result field must be a pointer or a sql.Null type.
//
//	type paymentWithAccount struct {
//	    ID          string
//	    Amount      int64
//	    AccountName *string // LEFT JOIN: NULL when the account is missing
//	}
//	err := database.Select[*Payment, paymentWithAccount](ctx,
//	    PaymentCols.ID, PaymentCols.Amount, AccountCols.Name.As("account_name")).
//	    Join(types.LeftJoin[*Account](AccountCols.Code.EqCol(PaymentCols.Account))).
//	    Scan(&rows)
//
// Rendered, every column qualified by its table because two tables are read:
//
//	SELECT `payments`.`id` AS `id`, `payments`.`amount` AS `amount`, `accounts`.`name` AS `account_name`
//	FROM `payments`
//	LEFT JOIN `accounts` ON `accounts`.`code` = `payments`.`account` AND `accounts`.`deleted_at` IS NULL
//	WHERE `payments`.`deleted_at` IS NULL
//
// Inside the ON a filter built from a plain name, FilterEq say, names a
// column of the joined model; a column of the query is named through its
// reference, which carries its table.
//
// ClickHouse carries no unique constraints, so a model join cannot be proved
// there and is rejected on a ClickHouse instance.
type JoinSource interface {
	sealedJoinSource()
}

// ModelJoin is a model joined on a unique key; see JoinSource. Join and
// LeftJoin build it.
type ModelJoin struct {
	// Model is an allocated instance of the joined model. It carries the
	// table name, the columns and the soft-delete scope, and the unique keys
	// the ON predicates are proved against.
	Model Model
	// Left keeps the rows of the query that match no joined row, with the
	// joined columns NULL: LEFT JOIN rather than JOIN.
	Left bool
	// On holds the ON predicates: the EqCol pairs tying the joined model to
	// the query, next to any condition narrowing the joined rows.
	On []Filter
}

func (ModelJoin) sealedJoinSource() {}

// Join joins model C on a unique key: JOIN, keeping only the rows of the
// query that match a row of C. The predicates are the ON condition:
//
//	types.Join[*Account](AccountCols.Code.EqCol(PaymentCols.Account), AccountCols.Tier.Eq("gold"))
//	// JOIN `accounts` ON `accounts`.`code` = `payments`.`account` AND `accounts`.`tier` = ?
//	//                    AND `accounts`.`deleted_at` IS NULL
func Join[C Model](on ...Filter) JoinSource {
	return modelJoin[C](on, false)
}

// LeftJoin joins model C on a unique key, keeping the rows of the query that
// match no row of C with the joined columns NULL: LEFT JOIN. The rules match
// Join; the result fields the joined columns bind to must hold NULL.
func LeftJoin[C Model](on ...Filter) JoinSource {
	return modelJoin[C](on, true)
}

// SelectJoin is a grouped select joined on its group keys as a derived
// table; see JoinSource. JoinSelect and LeftJoinSelect build it.
type SelectJoin struct {
	// Select is the joined select, a SelectBranch of some row type. It must
	// be grouped, and its group keys are what the ON pins.
	Select any
	// Left keeps the rows of the query that match no group, with the
	// select's terms NULL: LEFT JOIN rather than JOIN.
	Left bool
	// On holds the ON predicates: the EqCol pairs tying the select's group
	// keys, named through its model's column references, to the query.
	On []Filter
}

func (SelectJoin) sealedJoinSource() {}

// JoinSelect joins a grouped select as a derived table, JOIN (SELECT ...) AS
// jN ON ..., keeping only the rows of the query that match one of its
// groups. This is how a one-to-many relation is read beside its one side:
// the many side is grouped by the key first, so every key has one row, and
// that row is joined.
//
// The ON pins every group key of the select, each named through the select's
// model column reference and tied to a column of the query with Column.EqCol,
// or to a value. The query projects the select's terms by passing the very
// same terms, which then read as columns of the derived table:
//
//	orderTotal := OrderCols.Amount.Sum().As("order_total")
//	orders := database.Select[*Order, orderTotals](ctx, OrderCols.CustomerID.Group(), orderTotal)
//
//	err := database.Select[*Customer, customerOrders](ctx, CustomerCols.ID, CustomerCols.Name, orderTotal).
//	    Join(types.LeftJoinSelect(orders, OrderCols.CustomerID.EqCol(CustomerCols.ID))).
//	    Scan(&rows)
//	// SELECT `customers`.`id` AS `id`, `customers`.`name` AS `name`, `j0`.`order_total` AS `order_total`
//	// FROM `customers`
//	// LEFT JOIN (SELECT `customer_id` AS `customer_id`, COALESCE(SUM(`amount`), 0) AS `order_total`
//	//            FROM `orders` WHERE `orders`.`deleted_at` IS NULL GROUP BY `customer_id`) AS `j0`
//	//   ON `j0`.`customer_id` = `customers`.`id`
//	// WHERE `customers`.`deleted_at` IS NULL
//
// Only the select's terms are readable this way, passed as they are: its
// model's other columns are not columns of the derived table, and the term
// under another alias is not the term. A term the query could compute
// itself — one carrying no table, Count() say, or one of the queried table
// or of a model the query joins — is one spelling whether the query or the
// select wrote it: under its default alias, which two authors write
// independently, it is refused when the select projects it too, since the
// query's own reading of it would silently become the select's; an alias
// is written on purpose, so the select's aliased term passed to the query
// reads it through, and the query's own term keeps apart under an alias of
// its own. A term read this way may also key a window's partition or order
// it. In a grouped query a joined select's term
// is projected as a group key of the query, which is exact only when the
// query groups by the columns the select is joined on — the term is then
// constant within a group — and the framework requires it; a query with no
// measure of its own reads row-level, every row beside the select's terms.
// A condition on a joined select's measure belongs to that select's Having,
// and a condition on its rows to its Where: the ON names its keys, and an
// EXISTS subquery there has no row of the select's model to correlate with,
// so it is refused. The select carries no OrderBy, Limit or Offset of its
// own: a derived table has no use for them.
//
// One table backs at most one source of a query. A joined select is
// addressed through its model's column references, so a select over the
// queried table, or a second select over a table that already backs a
// source of the query, could not be told apart and is refused; the tables a
// select reads through joins of its own are its own to read. A per-row total
// over the queried table's own groups is a window instead,
// Sum().Over(PartitionBy(key)).
// A select grouped by a time bucket cannot be joined either: the bucket is a
// label of the column, not a value a column of the query equals. The select
// is tied by its own model's group keys, each column once: a key of a table
// the select joins has no spelling from outside and is refused, while a term
// the select reads from a select of its own is a column of the derived
// table, not a key.
//
// The derived table is materialized by the database, its rows being the
// groups of the select: a select narrowed by Where materializes fewer of
// them, so the conditions belong inside it rather than on the query around
// it.
func JoinSelect[R any](sub SelectBranch[R], on ...Filter) JoinSource {
	return SelectJoin{Select: sub, On: append([]Filter(nil), on...)}
}

// LeftJoinSelect joins a grouped select as a derived table, keeping the rows
// of the query that match none of its groups with the select's terms NULL:
// LEFT JOIN. The rules match JoinSelect; the result fields the select's terms
// bind to must hold NULL.
func LeftJoinSelect[R any](sub SelectBranch[R], on ...Filter) JoinSource {
	return SelectJoin{Select: sub, On: append([]Filter(nil), on...), Left: true}
}

// modelJoin allocates the joined model, the way subqueryFilter allocates a
// subquery's, so the database layer needs no type parameter to reach it.
// The predicates are copied: a caller appending to the slice it passed for
// a second join must not change the first one's ON.
func modelJoin[C Model](on []Filter, left bool) JoinSource {
	source := ModelJoin{On: append([]Filter(nil), on...), Left: left}
	typ := reflect.TypeFor[C]()
	if typ.Kind() == reflect.Pointer {
		if m, ok := reflect.TypeAssert[C](reflect.New(typ.Elem())); ok {
			source.Model = m
		}
	}
	return source
}
