package types

// AggregateFn is the function applied to one projection term. The set is
// closed, so a projection can never carry SQL the way a free-form select
// string could: the renderer maps each constant to a fixed expression and
// rejects anything else.
type AggregateFn string

const (
	// AggregateNone marks a group key rather than a measure. A projection term
	// without an aggregate function is what the framework derives GROUP BY
	// from, so the SELECT and GROUP BY lists can never disagree.
	AggregateNone          AggregateFn = ""
	AggregateCount         AggregateFn = "COUNT"
	AggregateCountDistinct AggregateFn = "COUNT_DISTINCT"
	AggregateSum           AggregateFn = "SUM"
	AggregateAvg           AggregateFn = "AVG"
	AggregateMin           AggregateFn = "MIN"
	AggregateMax           AggregateFn = "MAX"
)

// Valid reports whether the function is one this package defines. The renderer
// composes SQL from the constant, so a value from outside the set would reach
// the statement as text; the query builder rejects it instead.
func (f AggregateFn) Valid() bool {
	switch f {
	case AggregateNone, AggregateCount, AggregateCountDistinct,
		AggregateSum, AggregateAvg, AggregateMin, AggregateMax:
		return true
	default:
		return false
	}
}

// TimeBucket is the truncation granularity of a time group key. Bucketing is
// the one place where the same intent needs a different expression per
// dialect, so the constant travels through the builder and the database layer
// renders it; callers never see a format string.
type TimeBucket string

const (
	// TimeBucketNone groups by the raw column value.
	TimeBucketNone  TimeBucket = ""
	TimeBucketHour  TimeBucket = "hour"
	TimeBucketDay   TimeBucket = "day"
	TimeBucketMonth TimeBucket = "month"
)

// Valid reports whether the bucket is one this package defines. An unknown
// bucket would otherwise fall through to the day granularity and silently
// report the wrong period.
func (b TimeBucket) Valid() bool {
	switch b {
	case TimeBucketNone, TimeBucketHour, TimeBucketDay, TimeBucketMonth:
		return true
	default:
		return false
	}
}

// AggregateTerm is one term of an aggregate projection: a group key when Fn is
// AggregateNone, a measure otherwise.
//
// Terms are built through the generated column references
// (SampleCols.Amount.Sum()) or, for code that cannot name a concrete model,
// through the package-level string variants (SumOf("amount")). The typed path
// cannot express a function the column type does not support, because the
// generated reference does not carry the method; the string path names a
// column that is checked against the model schema when the query is built.
//
// A term never holds SQL. Column names are quoted by the database layer,
// values bind as statement parameters, and Fn and Bucket come from closed sets.
type AggregateTerm struct {
	// Fn is the aggregate function, or AggregateNone for a group key.
	Fn AggregateFn
	// Column is the snake case column name. It is empty only for COUNT(*).
	Column string
	// Bucket truncates a time group key. It is only meaningful when Fn is
	// AggregateNone and the column is a time column.
	Bucket TimeBucket
	// Conditions restrict a measure to the rows matching them, rendering as a
	// CASE expression inside the aggregate call. They reuse the query filter
	// tree, so conditional aggregation needs no predicate language of its own.
	Conditions []Filter
	// Alias names the term in the SELECT list and binds it to a field of the
	// result row. An empty alias defaults to the column name.
	Alias string
}

// IsMeasure reports whether the term is an aggregate rather than a group key.
func (t AggregateTerm) IsMeasure() bool { return t.Fn != AggregateNone }

// As renames the term in the SELECT list.
//
// It is optional. Every term already carries a default alias — the column name
// for a column term, "count" for COUNT(*) — so a projection whose result row
// fields are named after the columns needs no As at all:
//
//	Select(SampleCols.TenantID.Group(), SampleCols.Amount.Sum())
//	// scans into struct{ TenantID string; Amount int64 }
//
// Reach for As in the two cases the default cannot cover: when the result row
// field is named differently from the column, and when one projection carries
// two terms over the same column, whose default aliases would collide.
//
// The alias belongs to the result contract rather than to the column, which is
// why it is applied here instead of being a parameter of the constructors.
func (t AggregateTerm) As(alias string) AggregateTerm {
	t.Alias = alias
	return t
}

// Where restricts a measure to the rows matching filters, which is how a
// report projects several measures over different subsets in a single scan:
//
//	Count().Where(SampleCols.Status.Eq(StatusFailed)).As("failed")
//	// COUNT(CASE WHEN `status` = ? THEN 1 END) AS `failed`
//
// The filters are the ordinary query filters, including nested groups, so the
// same fail-closed rules and the same renderer apply.
func (t AggregateTerm) Where(filters ...Filter) AggregateTerm {
	t.Conditions = append(append([]Filter(nil), t.Conditions...), filters...)
	return t
}

// DefaultCountAlias is the alias COUNT(*) projects under when the caller does
// not rename it. A column term defaults to its column name, but COUNT(*) names
// no column, so without a default of its own it would be the one term that
// always had to be renamed.
const DefaultCountAlias = "count"

// Count counts rows: COUNT(*). It counts a row even when every column is NULL,
// which is what a plain row count means; use a column reference's Count for
// COUNT(column), which skips NULLs.
//
// It projects as "count" unless renamed with As.
func Count() AggregateTerm {
	return AggregateTerm{Fn: AggregateCount, Alias: DefaultCountAlias}
}

// The string-name variants below build the same terms from a plain column
// name, for code that cannot reference a concrete model: generic helpers and
// framework internals. They carry no Go type, so the column type rules are
// enforced against the model schema at build time rather than at compile time.
// This mirrors the existing split between Column.Asc and the Asc constructor.

// CountOf counts non-NULL values of a column.
func CountOf(column string) AggregateTerm {
	return AggregateTerm{Fn: AggregateCount, Column: column, Alias: column}
}

// CountDistinctOf counts distinct non-NULL values of a column.
func CountDistinctOf(column string) AggregateTerm {
	return AggregateTerm{Fn: AggregateCountDistinct, Column: column, Alias: column}
}

// SumOf adds up a numeric column.
func SumOf(column string) AggregateTerm {
	return AggregateTerm{Fn: AggregateSum, Column: column, Alias: column}
}

// AvgOf averages a numeric column.
func AvgOf(column string) AggregateTerm {
	return AggregateTerm{Fn: AggregateAvg, Column: column, Alias: column}
}

// MinOf returns the smallest value of a column.
func MinOf(column string) AggregateTerm {
	return AggregateTerm{Fn: AggregateMin, Column: column, Alias: column}
}

// MaxOf returns the largest value of a column.
func MaxOf(column string) AggregateTerm {
	return AggregateTerm{Fn: AggregateMax, Column: column, Alias: column}
}

// GroupOf groups by the raw value of a column.
func GroupOf(column string) AggregateTerm {
	return AggregateTerm{Column: column, Alias: column}
}

// ByHourOf, ByDayOf and ByMonthOf group a time column by a truncated bucket.
func ByHourOf(column string) AggregateTerm {
	return AggregateTerm{Column: column, Bucket: TimeBucketHour, Alias: column}
}

func ByDayOf(column string) AggregateTerm {
	return AggregateTerm{Column: column, Bucket: TimeBucketDay, Alias: column}
}

func ByMonthOf(column string) AggregateTerm {
	return AggregateTerm{Column: column, Bucket: TimeBucketMonth, Alias: column}
}

// HavingOp is a comparison applied to an aggregated value. Only the six
// orderings exist: the pattern and set operators of FilterOp have no meaning
// over a measure.
type HavingOp string

const (
	HavingOpEq  HavingOp = "eq"
	HavingOpNe  HavingOp = "ne"
	HavingOpGt  HavingOp = "gt"
	HavingOpGte HavingOp = "gte"
	HavingOpLt  HavingOp = "lt"
	HavingOpLte HavingOp = "lte"
)

// Valid reports whether the comparison is one this package defines. An unknown
// operator would otherwise fall through to equality and silently filter by the
// wrong comparison.
func (o HavingOp) Valid() bool {
	switch o {
	case HavingOpEq, HavingOpNe, HavingOpGt, HavingOpGte, HavingOpLt, HavingOpLte:
		return true
	default:
		return false
	}
}

// Having is one post-aggregation condition. It carries the term itself rather
// than an alias string, which has two consequences: a condition can never name
// a measure the projection did not declare, and the renderer can emit the full
// expression instead of the alias, which is required because PostgreSQL and
// SQL Server do not accept an output alias in HAVING.
type Having struct {
	Term  AggregateTerm
	Op    HavingOp
	Value any
}

// Eq, Ne, Gt, Gte, Lt and Lte build a post-aggregation condition on the term.
// The value type is checked when the query is built, because an aggregate's
// value type follows its function rather than its column: COUNT always yields
// an integer, AVG a float, and SUM widens.
func (t AggregateTerm) Eq(value any) Having { return Having{Term: t, Op: HavingOpEq, Value: value} }

func (t AggregateTerm) Ne(value any) Having { return Having{Term: t, Op: HavingOpNe, Value: value} }

func (t AggregateTerm) Gt(value any) Having { return Having{Term: t, Op: HavingOpGt, Value: value} }

func (t AggregateTerm) Gte(value any) Having { return Having{Term: t, Op: HavingOpGte, Value: value} }

func (t AggregateTerm) Lt(value any) Having { return Having{Term: t, Op: HavingOpLt, Value: value} }

func (t AggregateTerm) Lte(value any) Having { return Having{Term: t, Op: HavingOpLte, Value: value} }

// AggregateOrder is one ORDER BY term of an aggregate query. Unlike Order it
// sorts by a projection term, which is what a TopN report ranks by.
type AggregateOrder struct {
	Term      AggregateTerm
	Direction OrderDirection
}

// Asc and Desc sort the result rows by this term. An output alias is legal in
// ORDER BY on every supported dialect, so these render as the alias.
func (t AggregateTerm) Asc() AggregateOrder {
	return AggregateOrder{Term: t, Direction: OrderAsc}
}

func (t AggregateTerm) Desc() AggregateOrder {
	return AggregateOrder{Term: t, Direction: OrderDesc}
}
