package types

import (
	"github.com/hydroan/gst/internal/sse"
)

// Event is an alias for sse.Event.
// This allows users to use types.Event instead of importing internal/sse directly.
type Event = sse.Event

// ControllerConfig customizes how router.Register builds an internal handler for
// a route. It is the public configuration surface for controller behavior; the
// concrete controller handlers and their runtime state remain framework-owned.
type ControllerConfig[M Model] struct {
	// DB overrides the database handle used by the route. Only *gorm.DB is supported.
	DB any
	// TableName overrides the model table name used by the route.
	TableName string
	// ParamName names the route parameter that carries the resource ID.
	ParamName string
	// Route is the raw route string the handler is registered under. Controller
	// factories derive the service registry key from it, so it must match the
	// route passed to the corresponding service.Register call. router.Register
	// fills it in automatically; an empty route resolves no service and the
	// handler falls back to the no-op default service.
	Route string
}

// QueryConfig tunes how WithQuery turns a model value into WHERE conditions.
// The zero value means exact matching, AND combination, and the empty-query
// safety check enabled. See the WithQuery method for usage examples.
type QueryConfig struct {
	// FuzzyMatch switches model-field filtering from exact matching (single
	// value: IN, comma-separated values: IN list) to substring matching
	// (single value: LIKE '%v%', comma-separated values: REGEXP alternation
	// with special characters escaped).
	FuzzyMatch bool

	// AllowEmpty allows a query without any condition to match all records.
	// By default a nil model, a zero-value model, or all-empty field values
	// add the "1 = 0" safety condition instead, so a forgotten filter cannot
	// return or delete the whole table. RawQuery and FieldConditions count as
	// real conditions and disable the safety check on their own.
	AllowEmpty bool

	// UseOr combines the model-field conditions and RawQuery with OR instead
	// of AND. FieldConditions are not affected: they always join with AND.
	UseOr bool

	// RawQuery is a raw parameterized SQL fragment added as an extra WHERE
	// condition (OR-combined when UseOr is set). It works with a nil model
	// and combines with model-field conditions otherwise.
	RawQuery string

	// RawQueryArgs are the values bound to the RawQuery placeholders.
	RawQueryArgs []any

	// PresentFields marks columns whose filter values were explicitly provided
	// by the caller, keyed by snake case column name. Query construction treats
	// zero values (false, 0) of these columns as real conditions instead of
	// dropping them as unset, so a filter like "enabled=false" works. Columns
	// not listed here keep the default zero-value skip.
	PresentFields map[string]struct{}

	// FieldConditions are field-level operator filters ("field[op]=value")
	// combined with AND regardless of UseOr. They apply in every WithQuery
	// path, including nil/empty model queries, so List and Count stay
	// consistent. A condition with an unknown operator or empty column fails
	// closed: query construction adds "1 = 0" instead of dropping it.
	FieldConditions []FieldCondition
}

// SQLStatement contains a generated SQL statement in executable and rendered forms.
type SQLStatement struct {
	// Query is the parameterized SQL with placeholders.
	Query string
	// Args contains the values bound to Query.
	Args []any
	// RenderedSQL is dialect-rendered SQL for logging, inspection, and manual debugging.
	RenderedSQL string
}
