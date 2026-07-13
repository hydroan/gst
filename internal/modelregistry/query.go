package modelregistry

// Query declares framework-owned HTTP query parameters for list-style
// controller actions.
//
// Embedding Query in a model is an explicit opt-in: the model can still expose
// its own fields as query filters through query tags, while Query enables the
// standard List controls, including offset pagination, cursor pagination,
// fuzzy matching, sorting, expansion, and time-range filtering.
//
// Query intentionally covers only controls that keep list semantics intact.
// Controls that rewrite filter combination or tune query execution live in
// UnsafeQuery and require their own opt-in.
//
// Every field is ignored by JSON and GORM. The query tags are intentionally
// kept here because the List controller decodes URL query parameters into the
// request model before applying those controls. Database query construction must
// skip this struct so these controller-only values never become WHERE clauses.
type Query struct {
	Pagination
	Cursor

	Expand     *string `json:"-" gorm:"-" query:"_expand" url:"_expand,omitempty"`           // Expand lists model associations to preload, separated by commas.
	Depth      *uint   `json:"-" gorm:"-" query:"_depth" url:"_depth,omitempty"`             // Depth controls recursive expansion depth for expandable slice fields.
	Fuzzy      *bool   `json:"-" gorm:"-" query:"_fuzzy" url:"_fuzzy,omitempty"`             // Fuzzy switches model-field filtering from exact matching to LIKE/REGEXP matching.
	SortBy     string  `json:"-" gorm:"-" query:"_sortby" url:"_sortby,omitempty"`           // SortBy is the comma-separated order expression passed to WithOrder.
	ColumnName string  `json:"-" gorm:"-" query:"_column_name" url:"_column_name,omitempty"` // ColumnName selects the column used by StartTime and EndTime range filters.
	StartTime  string  `json:"-" gorm:"-" query:"_start_time" url:"_start_time,omitempty"`   // StartTime is the lower bound for the selected time-range column.
	EndTime    string  `json:"-" gorm:"-" query:"_end_time" url:"_end_time,omitempty"`       // EndTime is the upper bound for the selected time-range column.
}

// QueryEnabled marks models that opt in to general framework query parameters.
func (Query) QueryEnabled() {}

// Queryable is implemented by models that embed Query.
type Queryable interface {
	QueryEnabled()
}

// UnsafeQuery declares framework-owned HTTP query parameters that change how a
// list query is combined or executed, not just which rows it matches.
//
// These controls are escape hatches rather than regular list features:
//   - Or rewrites filter combination from AND to OR, which can defeat
//     mandatory service-level filters such as tenant or permission scoping.
//   - Index, Select, NoCache, and Nototal expose execution-level knobs
//     (index hints, column projection, cache bypass, count suppression).
//
// UnsafeQuery is therefore split from Query: embedding it is a separate,
// deliberate opt-in that signals the model owner accepts these risks.
// UnsafeQuery does not include Query; embed both when a model needs the
// regular List controls as well.
//
// Every field is ignored by JSON and GORM for the same reason as Query.
type UnsafeQuery struct {
	Or      *bool  `json:"-" gorm:"-" query:"_or" url:"_or,omitempty"`           // Or combines model-field filters with OR instead of AND when enabled.
	Index   string `json:"-" gorm:"-" query:"_index" url:"_index,omitempty"`     // Index requests a database index hint for the list query.
	Select  string `json:"-" gorm:"-" query:"_select" url:"_select,omitempty"`   // Select limits returned columns, separated by commas.
	NoCache bool   `json:"-" gorm:"-" query:"_nocache" url:"_nocache,omitempty"` // NoCache disables list cache reads and writes for this request.
	Nototal bool   `json:"-" gorm:"-" query:"_nototal" url:"_nototal,omitempty"` // Nototal skips the total-count query when true.
}

// UnsafeQueryEnabled marks models that opt in to unsafe framework query parameters.
func (UnsafeQuery) UnsafeQueryEnabled() {}

// UnsafeQueryable is implemented by models that embed UnsafeQuery.
type UnsafeQueryable interface {
	UnsafeQueryEnabled()
}

// Pagination declares offset-pagination query parameters for List actions.
//
// Embedding Pagination only enables page and size. It does not imply sorting,
// fuzzy matching, cursor pagination, or any other List query controls.
type Pagination struct {
	Page int `json:"-" gorm:"-" query:"page" url:"page,omitempty"` // Page is the one-based page number; non-positive values use the first page.
	Size int `json:"-" gorm:"-" query:"size" url:"size,omitempty"` // Size is the page size; negative values disable the limit.
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
	CursorValue  *string `json:"-" gorm:"-" query:"_cursor_value" url:"_cursor_value,omitempty"`   // CursorValue is the current cursor token for cursor pagination.
	CursorFields string  `json:"-" gorm:"-" query:"_cursor_fields" url:"_cursor_fields,omitempty"` // CursorFields names the cursor field; the first field is used today.
	CursorNext   bool    `json:"-" gorm:"-" query:"_cursor_next" url:"_cursor_next,omitempty"`     // CursorNext chooses the cursor direction; false requests the previous page.
}

// CursorEnabled marks models that opt in to cursor query parameters.
func (Cursor) CursorEnabled() {}

// Cursorable is implemented by models that embed Cursor.
type Cursorable interface {
	CursorEnabled()
}
