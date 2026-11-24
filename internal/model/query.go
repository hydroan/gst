package model

// Query declares framework-owned HTTP query parameters for list-style
// controller actions.
//
// Embedding Query in a model is an explicit opt-in: the model can still expose
// its own fields as query filters through query tags, while Query enables
// sorting, expansion, and field-level operator filters ("field[op]=value",
// covering substring matching via like/notlike and time or numeric ranges via
// the comparison operators).
//
// Paging is no part of Query. Every list pages by _page and _size with
// nothing to embed, and a model embedding Cursor has its list read by cursor
// instead; Query may be embedded beside Cursor, whose feed then expands and
// filters by operator but takes no _sort_by, the cursor defining its order.
//
// Every field is ignored by JSON and GORM. The query tags are intentionally
// kept here because the List controller decodes URL query parameters into the
// request model before applying those controls; GORM ignoring the fields is
// what keeps these controller-only values out of the WHERE clauses a query
// model builds.
type Query struct {
	Expand *string `json:"-" gorm:"-" query:"_expand" url:"_expand,omitempty"`   // Expand lists model associations to preload, separated by commas; a name the model does not expand is refused.
	Depth  *uint   `json:"-" gorm:"-" query:"_depth" url:"_depth,omitempty"`     // Depth is how many levels an expanded association holding the model itself, the children of a tree, is preloaded to, one to ten, zero meaning one and a larger value refused; any other association is one level.
	SortBy string  `json:"-" gorm:"-" query:"_sort_by" url:"_sort_by,omitempty"` // SortBy is the comma-separated ORDER BY expression; the List controller validates its columns and turns it into typed orders.
}

// queryEnabled marks models that opt in to general framework query parameters.
func (Query) queryEnabled() {}

// queryable is implemented by models that embed Query.
//
// The marker method is unexported, so embedding Query is the only way to
// satisfy queryable: models outside this package can neither declare the
// method themselves nor accidentally opt in without the Query fields.
// The other marker interface below, cursorable, is sealed the same way.
type queryable interface {
	queryEnabled()
}

// Cursor declares cursor-pagination query parameters for List actions.
//
// Embedding Cursor makes the list of the model a feed, read by cursor alone:
// it takes no _page, offset paging conflicting with cursor semantics, counts
// no total, and reads its first page from a request without a _cursor_value.
// Its batch size is _size, as the page size of any list.
//
// Cursor owns cursor position and direction only. Ordering for cursor
// pagination is derived from CursorField and CursorNext, so the feed takes no
// _sort_by, a second order source. The field named must be one no two rows
// share — the primary key, or a NOT NULL field carrying a unique index of
// its own — because the cursor is a single boundary value: on a shared one
// the rows holding it are split between pages and the ones a page had no
// room for are never read. Any other field is a 400.
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

// IsCursorable reports whether the list of m is a feed read by cursor, m
// embedding Cursor.
func IsCursorable(m any) bool {
	_, ok := m.(cursorable)
	return ok
}
