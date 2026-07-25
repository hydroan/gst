package types

// Column is a typed reference to a database column, generated per model by
// gg gen. T is the Go type of the column, so a filter built through a Column
// cannot name a column that does not exist nor bind a value of the wrong
// type: both mistakes stop at compile time instead of surfacing as a SQL
// error or a silently wrong result set.
//
// The methods are typed front ends for the FilterXxx constructors and produce
// exactly the same Filter values. Code that cannot reference a concrete model
// (generic helpers, framework internals, URL parsing) keeps using those
// constructors with a string column name.
type Column[T any] struct {
	// Name is the database column name resolved by gorm. It is also what the
	// APIs taking a plain column name (WithOrder, WithTimeRange) expect.
	Name string
}

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
