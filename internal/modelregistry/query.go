package modelregistry

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
// request model before applying those controls; GORM ignoring the fields is
// what keeps these controller-only values out of the WHERE clauses a query
// model builds.
type Query struct {
	Pagination
	Cursor

	Expand *string `json:"-" gorm:"-" query:"_expand" url:"_expand,omitempty"`   // Expand lists model associations to preload, separated by commas.
	Depth  *uint   `json:"-" gorm:"-" query:"_depth" url:"_depth,omitempty"`     // Depth is how many levels an expanded association holding the model itself, the children of a tree, is preloaded to; any other association is one level.
	SortBy string  `json:"-" gorm:"-" query:"_sort_by" url:"_sort_by,omitempty"` // SortBy is the comma-separated ORDER BY expression; the List controller validates its columns and turns it into typed orders.
}

// queryEnabled marks models that opt in to general framework query parameters.
func (Query) queryEnabled() {}

// queryable is implemented by models that embed Query.
//
// The marker method is unexported, so embedding Query is the only way to
// satisfy queryable: models outside this package can neither declare the
// method themselves nor accidentally opt in without the Query fields.
// The other marker interfaces below are sealed the same way.
type queryable interface {
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
//
// Neither parameter can be used to retrieve a whole table. A size the client
// does not set, or sets to zero or less, becomes the default page size; a size
// above the cap is clamped to it rather than refused, which is what keeps a
// client asking for everything from being served everything. Bulk retrieval is
// the Export action's job, and it is the only one that answers unbounded.
type Pagination struct {
	Page int `json:"-" gorm:"-" query:"_page" url:"_page,omitempty"` // Page is the one-based page number; non-positive values use the first page.
	Size int `json:"-" gorm:"-" query:"_size" url:"_size,omitempty"` // Size is the page size; non-positive values use the default page size and oversized ones clamp to the cap.
}

// paginationEnabled marks models that opt in to page and size query parameters.
func (Pagination) paginationEnabled() {}

// paginatable is implemented by models that embed Pagination.
type paginatable interface {
	paginationEnabled()
}

// Cursor declares cursor-pagination query parameters for List actions.
//
// Cursor owns cursor position and direction only. Ordering for cursor pagination
// is derived from CursorField and CursorNext, so SortBy intentionally remains
// outside this struct to avoid multiple competing order sources. The field
// named must be one no two rows share — the primary key, or a NOT NULL field
// carrying a unique index of its own — because the cursor is a single
// boundary value: on a shared one the rows holding it are split between pages
// and the ones a page had no room for are never read. Any other field is a
// 400. The feed is ordered from its first page on: a request with a cursor
// parameter but no _cursor_value reads the first page in the feed's order,
// and a model embedding Cursor alone reads it from a request naming no
// cursor parameter at all. Embedding Cursor also lets the client tune the
// batch size via _size (the field lives in Pagination; the controller reads
// it from the URL directly), while _page stays rejected: offset paging
// conflicts with cursor semantics.
type Cursor struct {
	CursorValue *string `json:"-" gorm:"-" query:"_cursor_value" url:"_cursor_value,omitempty"` // CursorValue is the current cursor token; it must parse as the cursor column's Go type. Unset, the request is the feed's first page.
	CursorField string  `json:"-" gorm:"-" query:"_cursor_field" url:"_cursor_field,omitempty"` // CursorField names the single field the cursor orders by: the primary key, which an unset one means, or a NOT NULL field with a unique index of its own.
	CursorNext  bool    `json:"-" gorm:"-" query:"_cursor_next" url:"_cursor_next,omitempty"`   // CursorNext chooses the cursor direction; false requests the previous page.
}

// cursorEnabled marks models that opt in to cursor query parameters.
func (Cursor) cursorEnabled() {}

// cursorable is implemented by models that embed Cursor.
type cursorable interface {
	cursorEnabled()
}

// IsQueryable reports whether m opted in to general framework query parameters
// by embedding Query.
func IsQueryable(m any) bool {
	_, ok := m.(queryable)
	return ok
}

// IsPaginatable reports whether m opted in to page and size query parameters
// by embedding Pagination directly or through Query.
func IsPaginatable(m any) bool {
	_, ok := m.(paginatable)
	return ok
}

// IsCursorable reports whether m opted in to cursor query parameters by
// embedding Cursor directly or through Query.
func IsCursorable(m any) bool {
	_, ok := m.(cursorable)
	return ok
}
