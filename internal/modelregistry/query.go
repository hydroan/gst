package modelregistry

// Query declares framework-owned HTTP query parameters for list-style
// controller actions.
//
// Embedding Query in a model is an explicit opt-in: the model can still expose
// its own fields as query filters through schema tags, while Query enables all
// standard List controls, including offset pagination, cursor pagination,
// fuzzy matching, sorting, expansion, cache control, and total-count
// suppression.
//
// Every field is ignored by JSON and GORM. The schema tags are intentionally
// kept here because the List controller decodes URL query parameters into the
// request model before applying those controls. Database query construction must
// skip this struct so these controller-only values never become WHERE clauses.
type Query struct {
	Pagination
	Cursor

	Expand     *string `json:"-" gorm:"-" schema:"_expand" url:"_expand,omitempty"`           // Expand lists model associations to preload, separated by commas.
	Depth      *uint   `json:"-" gorm:"-" schema:"_depth" url:"_depth,omitempty"`             // Depth controls recursive expansion depth for expandable slice fields.
	Fuzzy      *bool   `json:"-" gorm:"-" schema:"_fuzzy" url:"_fuzzy,omitempty"`             // Fuzzy switches model-field filtering from exact matching to LIKE/REGEXP matching.
	SortBy     string  `json:"-" gorm:"-" schema:"_sortby" url:"_sortby,omitempty"`           // SortBy is the comma-separated order expression passed to WithOrder.
	NoCache    bool    `json:"-" gorm:"-" schema:"_nocache" url:"_nocache,omitempty"`         // NoCache disables list cache reads and writes for this request.
	ColumnName string  `json:"-" gorm:"-" schema:"_column_name" url:"_column_name,omitempty"` // ColumnName selects the column used by StartTime and EndTime range filters.
	StartTime  string  `json:"-" gorm:"-" schema:"_start_time" url:"_start_time,omitempty"`   // StartTime is the lower bound for the selected time-range column.
	EndTime    string  `json:"-" gorm:"-" schema:"_end_time" url:"_end_time,omitempty"`       // EndTime is the upper bound for the selected time-range column.
	Or         *bool   `json:"-" gorm:"-" schema:"_or" url:"_or,omitempty"`                   // Or combines model-field filters with OR instead of AND when enabled.
	Index      string  `json:"-" gorm:"-" schema:"_index" url:"_index,omitempty"`             // Index requests a database index hint for the list query.
	Select     string  `json:"-" gorm:"-" schema:"_select" url:"_select,omitempty"`           // Select limits returned columns, separated by commas.
	Nototal    bool    `json:"-" gorm:"-" schema:"_nototal" url:"_nototal,omitempty"`         // Nototal skips the total-count query when true.
}

// QueryEnabled marks models that opt in to general framework query parameters.
func (Query) QueryEnabled() {}

// Queryable is implemented by models that embed Query.
type Queryable interface {
	QueryEnabled()
}

// Pagination declares offset-pagination query parameters for List actions.
//
// Embedding Pagination only enables page and size. It does not imply sorting,
// fuzzy matching, cursor pagination, or any other List query controls.
type Pagination struct {
	Page uint `json:"-" gorm:"-" schema:"page" url:"page,omitempty"` // Page is the one-based page number used by offset pagination.
	Size uint `json:"-" gorm:"-" schema:"size" url:"size,omitempty"` // Size is the page size used by offset pagination.
}

// PaginationEnabled marks models that opt in to page and size query parameters.
func (Pagination) PaginationEnabled() {}

// Paginatable is implemented by models that embed Pagination.
type Paginatable interface {
	PaginationEnabled()
}

// Cursor declares cursor-pagination query parameters for List actions.
//
// Cursor owns cursor position and direction only. Ordering for cursor pagination
// is derived from CursorFields and CursorNext, so SortBy intentionally remains
// outside this struct to avoid multiple competing order sources.
type Cursor struct {
	CursorValue  *string `json:"-" gorm:"-" schema:"_cursor_value" url:"_cursor_value,omitempty"`   // CursorValue is the current cursor token for cursor pagination.
	CursorFields string  `json:"-" gorm:"-" schema:"_cursor_fields" url:"_cursor_fields,omitempty"` // CursorFields names the cursor field; the first field is used today.
	CursorNext   bool    `json:"-" gorm:"-" schema:"_cursor_next" url:"_cursor_next,omitempty"`     // CursorNext chooses the cursor direction; false requests the previous page.
}

// CursorEnabled marks models that opt in to cursor query parameters.
func (Cursor) CursorEnabled() {}

// Cursorable is implemented by models that embed Cursor.
type Cursorable interface {
	CursorEnabled()
}
