package modelregistry

import (
	"reflect"
	"slices"
)

// Query declares framework-owned HTTP query parameters for list-style
// controller actions.
//
// Embedding Query in a model is an explicit opt-in: the model can still expose
// its own fields as query filters through query tags, while Query enables the
// standard List controls, including offset pagination, cursor pagination,
// sorting, expansion, and field-level operator filters ("field[op]=value",
// covering substring matching via like/notlike and time or numeric ranges via
// the comparison operators). Query already embeds Pagination and Cursor, so
// models that embed Query must not embed those structs again.
//
// Query covers every framework List control; there is no second opt-in to add.
//
// Every field is ignored by JSON and GORM. The query tags are intentionally
// kept here because the List controller decodes URL query parameters into the
// request model before applying those controls. Database query construction must
// skip this struct so these controller-only values never become WHERE clauses.
type Query struct {
	Pagination
	Cursor

	Expand *string `json:"-" gorm:"-" query:"_expand" url:"_expand,omitempty"`   // Expand lists model associations to preload, separated by commas.
	Depth  *uint   `json:"-" gorm:"-" query:"_depth" url:"_depth,omitempty"`     // Depth controls recursive expansion depth for expandable slice fields.
	SortBy string  `json:"-" gorm:"-" query:"_sort_by" url:"_sort_by,omitempty"` // SortBy is the comma-separated ORDER BY expression; the List controller validates its columns and turns it into typed orders.
}

// queryEnabled marks models that opt in to general framework query parameters.
func (Query) queryEnabled() {}

// Queryable is implemented by models that embed Query.
//
// The marker method is unexported, so embedding Query is the only way to
// satisfy Queryable: models outside this package can neither declare the
// method themselves nor accidentally opt in without the Query fields.
// The other marker interfaces below are sealed the same way.
type Queryable interface {
	queryEnabled()
}

// Pagination declares offset-pagination query parameters for List actions.
//
// Embedding Pagination only enables _page and _size. It does not imply
// sorting, fuzzy matching, cursor pagination, or any other List query
// controls.
//
// Like every framework-owned query parameter, _page and _size live in the
// "_" prefix namespace, so bare names such as page and size stay available
// as business filter fields on embedding models.
type Pagination struct {
	Page int `json:"-" gorm:"-" query:"_page" url:"_page,omitempty"` // Page is the one-based page number; non-positive values use the first page.
	Size int `json:"-" gorm:"-" query:"_size" url:"_size,omitempty"` // Size is the page size; negative values disable the limit.
}

// paginationEnabled marks models that opt in to page and size query parameters.
func (Pagination) paginationEnabled() {}

// Paginatable is implemented by models that embed Pagination.
type Paginatable interface {
	paginationEnabled()
}

// Cursor declares cursor-pagination query parameters for List actions.
//
// Cursor owns cursor position and direction only. Ordering for cursor pagination
// is derived from CursorField and CursorNext, so SortBy intentionally remains
// outside this struct to avoid multiple competing order sources. Embedding
// Cursor also lets the client tune the batch size via _size (the field lives
// in Pagination; the controller reads it from the URL directly), while _page
// stays rejected: offset paging conflicts with cursor semantics.
type Cursor struct {
	CursorValue *string `json:"-" gorm:"-" query:"_cursor_value" url:"_cursor_value,omitempty"` // CursorValue is the current cursor token; it must parse as the cursor column's Go type.
	CursorField string  `json:"-" gorm:"-" query:"_cursor_field" url:"_cursor_field,omitempty"` // CursorField names the single field the cursor orders by.
	CursorNext  bool    `json:"-" gorm:"-" query:"_cursor_next" url:"_cursor_next,omitempty"`   // CursorNext chooses the cursor direction; false requests the previous page.
}

// cursorEnabled marks models that opt in to cursor query parameters.
func (Cursor) cursorEnabled() {}

// Cursorable is implemented by models that embed Cursor.
type Cursorable interface {
	cursorEnabled()
}

// DefaultCursorColumn is the database column cursor pagination falls back to
// when the caller did not name one. It is the primary key of every framework
// base model, which is the only column guaranteed to be unique and monotonic.
// The database layer applies the fallback when building the boundary
// comparison, and URL parsing resolves the same column to type-check the
// cursor value, so both consumers read this single definition.
const DefaultCursorColumn = "id"

// IsQueryable reports whether m opted in to general framework query parameters
// by embedding Query.
func IsQueryable(m any) bool {
	_, ok := m.(Queryable)
	return ok
}

// IsPaginatable reports whether m opted in to page and size query parameters
// by embedding Pagination directly or through Query.
func IsPaginatable(m any) bool {
	_, ok := m.(Paginatable)
	return ok
}

// IsCursorable reports whether m opted in to cursor query parameters by
// embedding Cursor directly or through Query.
func IsCursorable(m any) bool {
	_, ok := m.(Cursorable)
	return ok
}

// queryMarkerTypes lists the sealed marker interfaces satisfied by the
// framework query structs above. Matching by capability instead of by concrete
// type keeps pointer embedding and nested marker structs working.
var queryMarkerTypes = []reflect.Type{
	reflect.TypeFor[Queryable](),
	reflect.TypeFor[Paginatable](),
	reflect.TypeFor[Cursorable](),
}

// IsQueryMarkerType reports whether t carries framework query parameters, that
// is, whether it or its pointer type satisfies one of the marker interfaces.
// Database query construction uses it to keep controller-only query fields out
// of SQL WHERE conditions.
func IsQueryMarkerType(t reflect.Type) bool {
	if t == nil {
		return false
	}
	if slices.ContainsFunc(queryMarkerTypes, t.Implements) {
		return true
	}
	if t.Kind() == reflect.Pointer {
		return false
	}
	return slices.ContainsFunc(queryMarkerTypes, reflect.PointerTo(t).Implements)
}
