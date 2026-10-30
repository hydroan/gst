package types

// The URL query parameters the framework owns, all in the "_" prefix
// namespace, so bare query keys always belong to model filter fields and
// never collide with framework controls: the page and size of a page, the
// limit of a read, the associations a Get expands and the depth it expands
// to, the ordering of a List, the format an Export writes, and the three
// parameters of cursor pagination.
const (
	QueryPage        = "_page"
	QuerySize        = "_size"
	QueryLimit       = "_limit"
	QueryExpand      = "_expand"
	QueryDepth       = "_depth"
	QuerySortBy      = "_sort_by"
	QueryFormat      = "_format"
	QueryCursorValue = "_cursor_value"
	QueryCursorField = "_cursor_field"
	QueryCursorNext  = "_cursor_next"
)
