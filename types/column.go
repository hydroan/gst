package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// AnyColumnRef is the type-erased view of every generated column reference,
// for options that take a heterogeneous column list: WithSelect accepts
// columns of different Go types in one call, which the parameterized
// ColumnRef cannot express. The unexported method keeps the set of
// implementations closed to the framework, so a stray type that happens to
// carry a Name method cannot slip into a column list.
type AnyColumnRef = itypes.AnyColumnRef

// TableNamer is the one method a column reference needs from its model: the
// table the column belongs to. Every Model satisfies it; the narrower
// interface lets a reference name its model as a type argument alone, with
// nothing else of the model contract in play.
type TableNamer = itypes.TableNamer

// ColumnRef is the shared typed view of every generated column reference.
// Helpers that accept a column take this interface rather than a concrete
// struct, because embedding is not subtyping in Go: NumericColumn[T] cannot
// be passed where Column[T] is expected, so a helper typed on the struct
// would reject exactly the numeric and time columns it is most often used
// with.
type ColumnRef[T any] = itypes.ColumnRef[T]

// Column is a typed reference to a database column, generated per model by
// gg gen. T is the Go type of the column, so a filter built through a Column
// cannot name a column that does not exist nor bind a value of the wrong
// type: both mistakes stop at compile time instead of surfacing as a SQL
// error or a silently wrong result set.
type Column[T any] = itypes.Column[T]

// NewColumn returns a typed reference to the named column of M's table. gg gen
// emits the calls in each model's generated file, naming the model as the
// first type argument, so the table comes from the model's own TableName and
// is never restated as a literal. Handwritten code, model hooks included,
// reads those generated Cols vars; gg check flags project code that mints a
// reference instead, with two exceptions. Generic code has no concrete model
// and so no Cols var: it names its type parameter as the model, and
// NewColumn[M, string]("id").In(ids...) keeps the value type checked where a
// plain column name would not. Module sources have no generated file and
// name their model the way gg gen does. The fields are unexported so a shared
// reference cannot be repointed at another column after construction.
func NewColumn[M TableNamer, T any](name string) Column[T] {
	return itypes.NewColumn[M, T](name)
}

// NumericColumn is the reference generated for a column whose Go type is
// numeric. It embeds Column, so every filter and order stays available, and
// adds the aggregate functions that only make sense over a number.
type NumericColumn[T any] = itypes.NumericColumn[T]

// NewNumericColumn returns the numeric reference to the named column of M's
// table, carrying Sum and Avg on top of everything Column has.
func NewNumericColumn[M TableNamer, T any](name string) NumericColumn[T] {
	return itypes.NewNumericColumn[M, T](name)
}

// TimeColumn is the reference generated for a time.Time column. It embeds
// Column and adds time bucketing, which is only meaningful over a time value
// and produces garbage rather than an error on some dialects when it is not.
type TimeColumn = itypes.TimeColumn

// NewTimeColumn returns the time reference to the named column of M's table,
// carrying the bucketing group keys on top of everything Column has.
func NewTimeColumn[M TableNamer](name string) TimeColumn {
	return itypes.NewTimeColumn[M](name)
}
