package types

import (
	"fmt"
	"reflect"
	"time"
)

// AnyColumnRef is the type-erased view of every generated column reference,
// for options that take a heterogeneous column list: WithSelect accepts
// columns of different Go types in one call, which the parameterized
// ColumnRef cannot express. The unexported method keeps the set of
// implementations closed to this package, so a stray type that happens to
// carry a Name method cannot slip into a column list.
type AnyColumnRef interface {
	// Name returns the database column name resolved by gorm.
	Name() string
	// Table returns the table the column belongs to, read from the model's
	// TableName when the reference was built.
	Table() string
	sealedAnyColumn()
}

// TableNamer is the one method a column reference needs from its model: the
// table the column belongs to. Every Model satisfies it; the narrower
// interface lets a reference name its model as a type argument alone, with
// nothing else of the model contract in play.
type TableNamer interface {
	TableName() string
}

// ColumnRef is the shared typed view of every generated column reference.
// Helpers that accept a column take this interface rather than a concrete
// struct, because embedding is not subtyping in Go: NumericColumn[T] cannot
// be passed where Column[T] is expected, so a helper typed on the struct
// would reject exactly the numeric and time columns it is most often used
// with.
//
// The type parameter is load-bearing. sealedColumn mentions T, so two column
// references only satisfy the same ColumnRef[T] when their Go types match,
// which is what makes tying a string column to an integer column with EqCol
// fail to compile. The method is also unexported, so the set of
// implementations stays closed to this package.
type ColumnRef[T any] interface {
	AnyColumnRef
	sealedColumn(T)
}

// Column is a typed reference to a database column, generated per model by
// gg gen. T is the Go type of the column, so a filter built through a Column
// cannot name a column that does not exist nor bind a value of the wrong
// type: both mistakes stop at compile time instead of surfacing as a SQL
// error or a silently wrong result set.
//
// A reference also knows the table it was generated for. That is what lets a
// read reject a column of another model up front: two models often share a
// column name, and a query reading the wrong one is valid SQL over the wrong
// table, which no schema check would catch.
//
// The methods are typed front ends for the FilterXxx, Asc, Desc and Assign
// constructors and produce the same Filter, Order and Assignment values, with
// the table the reference was built for filled in. Code that cannot reference
// a concrete model (generic helpers, framework internals) keeps using those
// constructors with a string column name.
//
// Columns whose Go type is numeric or time.Time are generated as NumericColumn
// or TimeColumn instead, which embed this type and add the functions that are
// only meaningful there.
type Column[T any] struct {
	table string
	name  string
}

// NewColumn returns a typed reference to the named column of M's table. gg gen
// emits the calls in each model's generated file, naming the model as the
// first type argument, so the table comes from the model's own TableName and
// is never restated as a literal. Handwritten code that cannot reference a
// generated Cols var should keep using the FilterXxx, Asc, Desc and Assign
// constructors with a plain column name instead of minting references; module
// sources, which have no generated file, mint them the same way with their
// model. The fields are unexported so a shared reference cannot be repointed
// at another column after construction.
//
// A virtual model, one embedding model.Empty, has no table and reports an
// empty TableName; its references carry no table and resolve to the table of
// whichever query reads them, the way the plain-name constructors do, which
// is all a resource without a table of its own can mean. An empty column
// name panics: references are constructed while generated code initializes
// its package-level Cols vars, so an incomplete one must not survive process
// startup.
func NewColumn[M TableNamer, T any](name string) Column[T] {
	if name == "" {
		panic(fmt.Sprintf("types: a column reference of %s requires a column name", reflect.TypeFor[M]()))
	}
	return Column[T]{table: tableNameOf[M](), name: name}
}

// tableNameOf reads the table name of M from a fresh value of it. A pointer
// model is instantiated through its pointee rather than left at its nil zero
// value, so TableName runs on a real value whatever receiver it declares: a
// value-receiver method reached through a nil pointer would dereference it.
func tableNameOf[M TableNamer]() string {
	var m M
	if typ := reflect.TypeFor[M](); typ.Kind() == reflect.Pointer {
		instance, ok := reflect.TypeAssert[M](reflect.New(typ.Elem()))
		if !ok {
			// Unreachable: New returns exactly the pointer type M names.
			panic("types: cannot instantiate the model of a column reference")
		}
		m = instance
	}
	return m.TableName()
}

// Name returns the database column name resolved by gorm. It is also what the
// order and cursor constructors taking a plain column name expect.
func (c Column[T]) Name() string { return c.name }

// Table returns the table the column belongs to, as the model's TableName
// reported it when the reference was built.
func (c Column[T]) Table() string { return c.table }

func (c Column[T]) sealedColumn(T) {}

func (c Column[T]) sealedAnyColumn() {}

// Eq matches rows where the column equals value.
func (c Column[T]) Eq(value T) Filter { return c.filter(FilterOpEq, value) }

// Ne matches rows where the column does not equal value.
func (c Column[T]) Ne(value T) Filter { return c.filter(FilterOpNe, value) }

// Gt matches rows where the column is greater than value.
func (c Column[T]) Gt(value T) Filter { return c.filter(FilterOpGt, value) }

// Gte matches rows where the column is greater than or equal to value.
func (c Column[T]) Gte(value T) Filter { return c.filter(FilterOpGte, value) }

// Lt matches rows where the column is less than value.
func (c Column[T]) Lt(value T) Filter { return c.filter(FilterOpLt, value) }

// Lte matches rows where the column is less than or equal to value.
func (c Column[T]) Lte(value T) Filter { return c.filter(FilterOpLte, value) }

// In matches rows where the column is one of values. Calling it without any
// value matches nothing, mirroring SQL list semantics.
func (c Column[T]) In(values ...T) Filter { return c.filter(FilterOpIn, append([]T(nil), values...)) }

// NotIn matches rows where the column is none of values. Calling it without
// any value matches nothing; it does not mean "exclude nothing".
func (c Column[T]) NotIn(values ...T) Filter {
	return c.filter(FilterOpNotIn, append([]T(nil), values...))
}

// Like matches rows where the column contains value as a substring. The
// pattern is a string on every column type, because substring matching runs
// against the database's string rendering of the value.
func (c Column[T]) Like(value string) Filter { return c.filter(FilterOpLike, value) }

// NotLike matches rows where the column does not contain value as a substring.
func (c Column[T]) NotLike(value string) Filter { return c.filter(FilterOpNotLike, value) }

// StartsWith matches rows where the column starts with value.
func (c Column[T]) StartsWith(value string) Filter { return c.filter(FilterOpStartsWith, value) }

// EndsWith matches rows where the column ends with value.
func (c Column[T]) EndsWith(value string) Filter { return c.filter(FilterOpEndsWith, value) }

// IsNull matches rows where the column is NULL.
func (c Column[T]) IsNull() Filter { return c.filter(FilterOpIsNull, true) }

// IsNotNull matches rows where the column is not NULL.
func (c Column[T]) IsNotNull() Filter { return c.filter(FilterOpIsNull, false) }

// Regex matches rows where the column matches the regular expression expr.
func (c Column[T]) Regex(expr string) Filter { return c.filter(FilterOpRegex, expr) }

// NotRegex matches rows where the column does not match the regular
// expression expr.
func (c Column[T]) NotRegex(expr string) Filter { return c.filter(FilterOpNotRegex, expr) }

// JSONContains matches rows whose JSON array column contains value.
func (c Column[T]) JSONContains(value string) Filter { return c.filter(FilterOpJSONContains, value) }

// EqCol ties this column to parent, a column of another table of the query:
// the enclosing query's model inside a subquery, the queried model or an
// earlier joined one inside a join; see FilterEqCol. Both must be columns of
// the same Go type, and the predicate carries both tables, so inside a join
// either column may be written first. A nil parent leaves the other side
// empty, and the predicate then fails closed.
//
// The Col suffix says that the argument is a column rather than a value,
// which Eq takes; Go has no overloading to tell the two apart by type, and
// gorm gen spells the same comparison the same way.
func (c Column[T]) EqCol(parent ColumnRef[T]) Filter {
	if parent == nil {
		return c.filter(FilterOpEqCol, "")
	}
	return c.filter(FilterOpEqCol, parent)
}

// filter builds a filter on this column, carrying the table the reference
// was built for.
func (c Column[T]) filter(op FilterOp, value any) Filter {
	return Filter{Table: c.table, Column: c.name, Op: op, Value: value}
}

// Asc orders by the column ascending. The order carries the table the
// reference was built for, which the chain's reads and a select check: a
// select that joins tells two tables' columns of one name apart by it, and a
// chain refuses an order of another model; see Order.
func (c Column[T]) Asc() Order { return Order{Table: c.table, Column: c.name, Direction: OrderAsc} }

// Desc orders by the column descending; see Asc.
func (c Column[T]) Desc() Order { return Order{Table: c.table, Column: c.name, Direction: OrderDesc} }

// Set assigns value to the column, the unit UpdateByID accepts. The value is
// typed by the column, so a wrong-typed value or a misspelled column fails to
// compile; the assignment carries the table the reference was built for, so
// a column of another model, which may well share the name, is refused when
// the write is built.
func (c Column[T]) Set(value T) Assignment {
	return Assignment{Table: c.table, Column: c.name, Value: value}
}

// The projection methods below turn the column into a Term. Every column
// carries the ones that cannot be silently wrong on any column type; the
// functions that are silently wrong on the wrong type live on the specialized
// references instead: a database answers SUM over a text column with 0 rather
// than an error on both MySQL and SQLite, which reaches a report as a wrong
// number.

// Group makes this column a group key of the projection. The framework derives
// GROUP BY from the group keys, so a projection cannot disagree with its own
// GROUP BY list. Over a nullable column the rows without a value form a group
// of their own, keyed NULL, so the result field is a pointer or sql.Null type
// unless a condition on the column in Where keeps those rows out.
func (c Column[T]) Group() Term { return c.term(FnNone) }

// exprTerm projects the column as it is stored, which is what passing a
// column reference to Select directly means.
func (c Column[T]) exprTerm() Term {
	term := c.term(FnNone)
	term.Plain = true
	return term
}

// As projects the column as it is stored under alias, the way a column
// reference passed to Select directly projects under its own name. It aligns
// a column with a differently named field of the result row, which a union
// branch needs when the models it stacks spell a column differently:
//
//	RefundCols.SettledAt.As("created_at")
//	// `settled_at` AS `created_at`
//
// A group key is renamed on the term instead: Cols.X.Group().As("y"). An
// empty alias changes nothing, as on a term.
func (c Column[T]) As(alias string) Term { return c.exprTerm().As(alias) }

// Count counts the rows whose value of this column is not NULL. Use the
// package-level Count for COUNT(*), which counts every row.
func (c Column[T]) Count() Term { return c.term(FnCount) }

// CountDistinct counts the distinct non-NULL values of this column.
func (c Column[T]) CountDistinct() Term { return c.term(FnCountDistinct) }

// Min returns the smallest value of this column. It yields NULL for a group
// with no non-NULL value, so the result row field must be a pointer.
func (c Column[T]) Min() Term { return c.term(FnMin) }

// Max returns the largest value of this column. The NULL rules match Min.
func (c Column[T]) Max() Term { return c.term(FnMax) }

// Lag reads this column from the previous row of the window, in the window's
// order; the first row of each partition has no previous row and yields NULL,
// so the result field must be a pointer. It only exists over an ordered
// window; see Term.Over.
func (c Column[T]) Lag() Term { return c.term(FnLag) }

// Lead reads this column from the next row of the window; the last row of
// each partition yields NULL. The rules match Lag.
func (c Column[T]) Lead() Term { return c.term(FnLead) }

// term builds the projection term the methods above share: the column with
// its table, aliased by its own name until As renames it.
func (c Column[T]) term(fn TermFn) Term {
	return Term{Fn: fn, Table: c.table, Column: c.name, Alias: c.name}
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

// NewNumericColumn returns the numeric reference to the named column of M's
// table, carrying Sum and Avg on top of everything Column has.
func NewNumericColumn[M TableNamer, T any](name string) NumericColumn[T] {
	return NumericColumn[T]{Column: NewColumn[M, T](name)}
}

// Sum adds up this column. The renderer wraps it in COALESCE(..., 0) so an
// empty group sums to zero rather than scanning NULL into the result row.
func (c NumericColumn[T]) Sum() Term { return c.term(FnSum) }

// Avg averages this column. It yields NULL for a group with no non-NULL value
// and is never coalesced, because a zero average and no data are different
// answers; the result row field must be a pointer.
func (c NumericColumn[T]) Avg() Term { return c.term(FnAvg) }

// TimeColumn is the reference generated for a time.Time column. It embeds
// Column and adds time bucketing, which is only meaningful over a time value
// and produces garbage rather than an error on some dialects when it is not.
type TimeColumn struct {
	Column[time.Time]
}

// NewTimeColumn returns the time reference to the named column of M's table,
// carrying the bucketing group keys on top of everything Column has.
func NewTimeColumn[M TableNamer](name string) TimeColumn {
	return TimeColumn{Column: NewColumn[M, time.Time](name)}
}

// ByHour, ByDay and ByMonth make this column a group key truncated to the
// bucket, which is what a trend report groups by. The truncation expression
// differs per dialect and is rendered by the database layer, so callers never
// deal with a format string. The bucket of NULL is NULL: over a nullable
// column the rows without a value form a group of their own, so the result
// field is a pointer or sql.Null type unless a condition on the column in
// Where keeps those rows out.
func (c TimeColumn) ByHour() Term { return c.bucket(TimeBucketHour) }

func (c TimeColumn) ByDay() Term { return c.bucket(TimeBucketDay) }

func (c TimeColumn) ByMonth() Term { return c.bucket(TimeBucketMonth) }

func (c TimeColumn) bucket(bucket TimeBucket) Term {
	term := c.term(FnNone)
	term.Bucket = bucket
	return term
}
