package urlquery

import (
	"net/url"
	"strconv"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// defaultLimit is the full-table safety bottom line for list queries whose
// model exposes no client-adjustable page size.
const defaultLimit = 1000

// defaultPageSize and maxPageSize bound the _size parameter on models that
// embed Pagination or Cursor: an unset size defaults to a small first page
// and oversized values clamp to the cap instead of erroring, matching common
// API practice (bulk retrieval belongs to the Export action).
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// Pagination returns the page and size arguments of the request, ready to be
// passed to Database.WithPagination.
//
// A model opts in to client-controlled paging by embedding model.Pagination
// (page and size) or model.Cursor (size only); a parameter the model did not
// opt in to is ignored and falls back to the framework default. The returned
// page is always at least 1 — an unset, non-positive or unparsable page means
// the first page — so a caller can compute offsets or slice in-memory pages
// without normalizing again. An unset size defaults to a small first page and
// an oversized one clamps to the cap, while a model without client size
// control keeps the full-table safety limit. An active cursor resets page to
// 1 so offset paging cannot stack on top of cursor filtering.
func Pagination(q url.Values, m types.Model) (page, size int) {
	paginatable := modelregistry.IsPaginatable(m)
	cursorable := modelregistry.IsCursorable(m)

	if paginatable {
		page, _ = strconv.Atoi(q.Get(consts.QUERY_PAGE))
	}
	if page <= 0 {
		page = 1
	}
	if paginatable || cursorable {
		size, _ = strconv.Atoi(q.Get(consts.QUERY_SIZE))
		switch {
		case size <= 0:
			size = defaultPageSize
		case size > maxPageSize:
			size = maxPageSize
		}
	} else {
		size = defaultLimit
	}
	if cursorable && len(q.Get(consts.QUERY_CURSOR_VALUE)) > 0 {
		page = 1
	}
	return page, size
}
