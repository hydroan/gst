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
}

// QueryConfig configures the behavior of WithQuery method.
//
// Fields:
//
//   - FuzzyMatch: Enable fuzzy matching (LIKE/REGEXP queries). Default: false (exact match with IN clause)
//
//   - Single value: Uses LIKE pattern (WHERE name LIKE '%value%')
//
//   - Multiple values (comma-separated): Uses REGEXP pattern (WHERE name REGEXP '.*value1.*|.*value2.*')
//
//   - Empty strings in comma-separated values are automatically skipped
//
//   - REGEXP special characters are automatically escaped
//
//   - Note: REGEXP may not be available in all databases (e.g., SQLite requires extension)
//
//   - AllowEmpty: Allow empty query conditions to match all records. Default: false (blocked for safety)
//
//   - When false: Empty queries are blocked (adds WHERE 1 = 0)
//
//   - When true: Empty queries match all records (full table scan)
//
//   - Empty query cases: nil, empty struct, all fields are zero values, all field values are empty strings
//
//   - Critical: Use with caution, especially for Delete operations
//
//   - UseOr: Use OR logic to combine query conditions. Default: false (uses AND logic)
//
//   - When false: Multiple fields use AND logic (WHERE name IN ('John') AND age IN (18))
//
//   - When true: Multiple fields use OR logic (WHERE name IN ('John') OR age IN (18))
//
//   - First condition always uses WHERE, subsequent conditions use OR
//
//   - Works with both exact match and fuzzy match
//
//   - RawQuery: Raw SQL query string for custom WHERE conditions. When provided, model fields are ignored
//
//   - Works even when query is nil
//
//   - Supports parameterized queries with RawQueryArgs
//
//   - Example: "age > ? AND status = ?"
//
//   - RawQueryArgs: Arguments for the raw SQL query, used with RawQuery for parameterized queries
//
//   - Can be nil or empty slice if RawQuery has no placeholders
//
//   - Example: []any{18, "active"}
//
// CRITICAL SAFETY FEATURE:
// Empty query conditions (all fields are zero values) are blocked by default to prevent
// catastrophic data loss scenarios, especially when the result is used for Delete operations.
//
// Empty Query Examples:
//   - WithQuery(&User{})                    → all fields are zero values
//   - WithQuery(&User{Name: "", Email: ""}) → all field values are empty strings
//   - WithQuery(&KV{Key: ""})               → happens when removed slice is empty
//
// Usage Examples:
//
//	// Exact match (default)
//	WithQuery(&User{Name: "John"})
//	WithQuery(&User{Name: "John"}, QueryConfig{})
//
//	// Exact match with multiple values (comma-separated)
//	WithQuery(&User{Name: "John,Jack"})  // WHERE name IN ('John', 'Jack')
//	WithQuery(&User{ID: "id1,id2,id3"})  // WHERE id IN ('id1', 'id2', 'id3')
//
//	// Fuzzy match - single value (LIKE)
//	WithQuery(&User{Name: "John"}, QueryConfig{FuzzyMatch: true})  // WHERE name LIKE '%John%'
//
//	// Fuzzy match - multiple values (REGEXP)
//	WithQuery(&User{Name: "John,Jack"}, QueryConfig{FuzzyMatch: true})  // WHERE name REGEXP '.*John.*|.*Jack.*'
//
//	// Allow empty query (ListFactory with pagination)
//	WithQuery(&User{}, QueryConfig{AllowEmpty: true})  // Returns all records
//
//	// Fuzzy match + Allow empty
//	WithQuery(&User{}, QueryConfig{FuzzyMatch: true, AllowEmpty: true})
//
//	// Use OR logic to combine conditions
//	WithQuery(&User{Name: "John", Email: "john@example.com"}, QueryConfig{UseOr: true})
//	// WHERE name IN ('John') OR email IN ('john@example.com')
//
//	// OR logic with fuzzy match
//	WithQuery(&User{Name: "John", Email: "example"}, QueryConfig{UseOr: true, FuzzyMatch: true})
//	// WHERE name LIKE '%John%' OR email LIKE '%example%'
//
//		// Raw SQL query (can be combined with model fields using AND logic)
//	WithQuery(&User{}, QueryConfig{RawQuery: "age > ? AND status = ?", RawQueryArgs: []any{18, "active"}})
//	WithQuery(nil, QueryConfig{RawQuery: "created_at BETWEEN ? AND ?", RawQueryArgs: []any{startDate, endDate}})
//
//	// Raw SQL with complex conditions
//	WithQuery(&User{}, QueryConfig{RawQuery: "created_at BETWEEN ? AND ? OR priority IN (?)", RawQueryArgs: []any{startDate, endDate, priorities}})
//	// Raw SQL combined with model fields
//	WithQuery(&User{Name: "John"}, QueryConfig{RawQuery: "age > ?", RawQueryArgs: []any{18}})  // WHERE age > ? AND name IN ('John')
//
//	// Combined options
//	WithQuery(&User{Name: "John"}, QueryConfig{
//	    FuzzyMatch: true,
//	    UseOr:      true,
//	    AllowEmpty: false,
//	})
type QueryConfig struct {
	FuzzyMatch   bool   // Enable fuzzy matching (LIKE/REGEXP). Default: false
	AllowEmpty   bool   // Allow empty query conditions. Default: false
	UseOr        bool   // Use OR logic to combine query conditions. Default: false (uses AND)
	RawQuery     string // Raw SQL query string for custom WHERE conditions
	RawQueryArgs []any  // Arguments for the raw SQL query parameters

	// PresentFields marks columns whose filter values were explicitly provided
	// by the caller, keyed by snake case column name. Query construction treats
	// zero values (false, 0) of these columns as real conditions instead of
	// dropping them as unset, so a filter like "enabled=false" works. Columns
	// not listed here keep the default zero-value skip.
	PresentFields map[string]struct{}
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
