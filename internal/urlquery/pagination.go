package urlquery

import (
	"net/url"
	"strconv"

	"github.com/hydroan/gst/internal/gst"
)

// DefaultPageSize and MaxPageSize bound the _size parameter of every list:
// a size left out, or zero, is a small first page, and one past the cap is
// refused rather than brought down to it, so a client is never answered
// fewer records than it asked for without being told (bulk retrieval belongs
// to the Export action). The pb generator quotes both in the comment of a
// List request's size field, so a gRPC caller reads the bounds off the
// message.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Pagination returns the page and size arguments of the request, ready to be
// passed to Database.WithPagination.
//
// Every list pages: it reads its size from _size, DefaultPageSize when the
// parameter is left out or zero, and its page from _page, the first one when
// left out or zero, so a caller can compute offsets or slice in-memory pages
// without normalizing again. A feed, the list of a model embedding
// model.Cursor, has no page to read: Cursor refuses the request of one
// carrying _page, so offset paging cannot stack on top of cursor filtering.
//
// Pagination has no error to answer with, so what it reads is a count within
// its cap already: an HTTP request whose _page or _size is no integer, a
// negative one, or a size past MaxPageSize is refused at the door (see the
// strictQuery middleware, which reads both by ParseCount), and so is the
// size of a gRPC request message past the cap, its fields being unsigned.
func Pagination(q url.Values) (page, size int) {
	// A parameter left out is not parsed: strconv allocates the error it
	// refuses an empty string with.
	if raw := q.Get(gst.QueryPage); raw != "" {
		page, _ = strconv.Atoi(raw)
	}
	if page <= 0 {
		page = 1
	}
	if raw := q.Get(gst.QuerySize); raw != "" {
		size, _ = strconv.Atoi(raw)
	}
	if size <= 0 {
		size = DefaultPageSize
	}
	return page, size
}
