package types

import "time"

// ColumnRef is the shared view of every generated column reference. Helpers
// that accept a column take this interface rather than a concrete struct,
// because embedding is not subtyping in Go: NumericColumn[T] cannot be passed
// where Column[T] is expected, so a helper typed on the struct would reject
// exactly the numeric and time columns it is most often used with.
//
// The type parameter is load-bearing. sealedColumn mentions T, so two column
// references only satisfy the same ColumnRef[T] when their Go types match,
// which is what makes correlating a string column with an integer column fail
// to compile. The method is also unexported, so the set of implementations
// stays closed to this package.
type ColumnRef[T any] interface {
	// ColumnName returns the database column name resolved by gorm.
	ColumnName() string
	sealedColumn(T)
}

// Column is a typed reference to a database column, generated per model by
// gg gen. T is the Go type of the column, so a filter built through a Column
// cannot name a column that does not exist nor bind a value of the wrong
// type: both mistakes stop at compile time instead of surfacing as a SQL
// error or a silently wrong result set.
//
// The methods are typed front ends for the FilterXxx, Asc and Desc
// constructors and produce exactly the same Filter and Order values. Code that
// cannot reference a concrete model (generic helpers, framework internals, URL
// parsing) keeps using those constructors with a string column name.
//
// Columns whose Go type is numeric or time.Time are generated as NumericColumn
// or TimeColumn instead, which embed this type and add the aggregate methods
// that are only meaningful there.
type Column[T any] struct {
	// Name is the database column name resolved by gorm. It is also what the
	// order and cursor constructors taking a plain column name expect.
	Name string
}

// ColumnName returns the database column name.
func (c Column[T]) ColumnName() string { return c.Name }

func (c Column[T]) sealedColumn(T) {}

// Eq matches rows where the column equals value.
func (c Column[T]) Eq(value T) Filter { return FilterEq(c.Name, value) }

// Ne matches rows where the column does not equal value.
func (c Column[T]) Ne(value T) Filter { return FilterNe(c.Name, value) }

// Gt matches rows where the column is greater than value.
func (c Column[T]) Gt(value T) Filter { return FilterGt(c.Name, value) }

// Gte matches rows where the column is greater than or equal to value.
func (c Column[T]) Gte(value T) Filter { return FilterGte(c.Name, value) }

// Lt matches rows where the column is less than value.
func (c Column[T]) Lt(value T) Filter { return FilterLt(c.Name, value) }

// Lte matches rows where the column is less than or equal to value.
func (c Column[T]) Lte(value T) Filter { return FilterLte(c.Name, value) }

// In matches rows where the column is one of values. Calling it without any
// value matches nothing, mirroring SQL list semantics.
func (c Column[T]) In(values ...T) Filter { return FilterIn(c.Name, values) }

// NotIn matches rows where the column is none of values. Calling it without
// any value matches nothing; it does not mean "exclude nothing".
func (c Column[T]) NotIn(values ...T) Filter { return FilterNotIn(c.Name, values) }

// Like matches rows where the column contains value as a substring. The
// pattern is a string on every column type, because substring matching runs
// against the database's string rendering of the value.
func (c Column[T]) Like(value string) Filter { return FilterLike(c.Name, value) }

// NotLike matches rows where the column does not contain value as a substring.
func (c Column[T]) NotLike(value string) Filter { return FilterNotLike(c.Name, value) }

// StartsWith matches rows where the column starts with value.
func (c Column[T]) StartsWith(value string) Filter { return FilterStartsWith(c.Name, value) }

// EndsWith matches rows where the column ends with value.
func (c Column[T]) EndsWith(value string) Filter { return FilterEndsWith(c.Name, value) }

// IsNull matches rows where the column is NULL.
func (c Column[T]) IsNull() Filter { return FilterIsNull(c.Name) }

// NotNull matches rows where the column is not NULL.
func (c Column[T]) NotNull() Filter { return FilterNotNull(c.Name) }

// Regex matches rows where the column matches the regular expression expr.
func (c Column[T]) Regex(expr string) Filter { return FilterRegex(c.Name, expr) }

// NotRegex matches rows where the column does not match the regular
// expression expr.
func (c Column[T]) NotRegex(expr string) Filter { return FilterNotRegex(c.Name, expr) }

// JSONContains matches rows whose JSON array column contains value.
func (c Column[T]) JSONContains(value string) Filter { return FilterJSONContains(c.Name, value) }

// Asc orders by the column ascending.
func (c Column[T]) Asc() Order { return Asc(c.Name) }

// Desc orders by the column descending.
func (c Column[T]) Desc() Order { return Desc(c.Name) }

// The aggregate methods below are the ones that cannot be silently wrong on
// any column type, so every column carries them. Functions that are silently
// wrong on the wrong type live on the specialized references instead: a
// database answers SUM over a text column with 0 rather than an error on both
// MySQL and SQLite, which reaches a report as a wrong number.

// Count counts the rows whose value of this column is not NULL. Use the
// package-level Count for COUNT(*), which counts every row.
func (c Column[T]) Count() AggregateTerm { return c.term(AggregateCount) }

// CountDistinct counts the distinct non-NULL values of this column.
func (c Column[T]) CountDistinct() AggregateTerm { return c.term(AggregateCountDistinct) }

// Min returns the smallest value of this column. It yields NULL for a group
// with no non-NULL value, so the result row field must be a pointer.
func (c Column[T]) Min() AggregateTerm { return c.term(AggregateMin) }

// Max returns the largest value of this column. The NULL rules match Min.
func (c Column[T]) Max() AggregateTerm { return c.term(AggregateMax) }

// Group makes this column a group key of the projection. The framework derives
// GROUP BY from the group keys, so a projection cannot disagree with its own
// GROUP BY list.
func (c Column[T]) Group() AggregateTerm { return c.term(AggregateNone) }

func (c Column[T]) term(fn AggregateFn) AggregateTerm {
	return AggregateTerm{Fn: fn, Column: c.Name, Alias: c.Name}
}

// NumericColumn is the reference generated for a column whose Go type is
// numeric. It embeds Column, so every filter and order stays available, and
// adds the aggregate functions that only make sense over a number.
//
// The specialization exists because of how the mistake fails, not because of
// tidiness: SUM over a text column returns 0 with a warning on MySQL and
// SQLite, so it surfaces as a plausible-looking wrong number on a dashboard
// rather than as an error. Functions whose misuse is merely useless rather
// than silently wrong stay on Column.
type NumericColumn[T any] struct {
	Column[T]
}

// Sum adds up this column. The renderer wraps it in COALESCE(..., 0) so an
// empty group sums to zero rather than scanning NULL into the result row.
func (c NumericColumn[T]) Sum() AggregateTerm { return c.term(AggregateSum) }

// Avg averages this column. It yields NULL for a group with no non-NULL value
// and is never coalesced, because a zero average and no data are different
// answers; the result row field must be a pointer.
func (c NumericColumn[T]) Avg() AggregateTerm { return c.term(AggregateAvg) }

// TimeColumn is the reference generated for a time.Time column. It embeds
// Column and adds time bucketing, which is only meaningful over a time value
// and produces garbage rather than an error on some dialects when it is not.
type TimeColumn struct {
	Column[time.Time]
}

// ByHour, ByDay and ByMonth make this column a group key truncated to the
// bucket, which is what a trend report groups by. The truncation expression
// differs per dialect and is rendered by the database layer, so callers never
// deal with a format string.
func (c TimeColumn) ByHour() AggregateTerm { return c.bucket(TimeBucketHour) }

func (c TimeColumn) ByDay() AggregateTerm { return c.bucket(TimeBucketDay) }

func (c TimeColumn) ByMonth() AggregateTerm { return c.bucket(TimeBucketMonth) }

func (c TimeColumn) bucket(bucket TimeBucket) AggregateTerm {
	term := c.term(AggregateNone)
	term.Bucket = bucket
	return term
}
