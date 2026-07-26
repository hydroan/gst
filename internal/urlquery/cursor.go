package urlquery

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// Cursor returns the cursor position of the request, ready to be passed to
// Database.WithCursor.
//
// A model opts in to cursor pagination by embedding model.Cursor; any other
// model yields a zero cursor, which WithCursor treats as a no-op. The cursor
// column is validated against the model's filterable columns, so an unknown
// column fails here instead of reaching the database.
//
// A URL cursor always pages an ascending feed: _cursor_next only chooses
// whether the request travels along the feed or back down it. A descending
// feed is a service-side cursor, built with types.CursorForward on a Desc
// order.
func Cursor(q url.Values, m types.Model) (types.Cursor, error) {
	if !modelregistry.IsCursorable(m) {
		return types.Cursor{}, nil
	}
	value := q.Get(consts.QUERY_CURSOR_VALUE)
	if len(value) == 0 {
		return types.Cursor{}, nil
	}

	// An unnamed column leaves the order column empty on purpose: the database
	// layer owns the primary key fallback, so both this path and a service
	// building a cursor by hand land on the same default.
	column := ""
	if field := strings.TrimSpace(q.Get(consts.QUERY_CURSOR_FIELD)); len(field) > 0 {
		columns, err := filterableColumns(m)
		if err != nil {
			return types.Cursor{}, err
		}
		resolved, ok := columns[field]
		if !ok {
			return types.Cursor{}, errors.Newf("unknown cursor column %q", field)
		}
		column = resolved.DBName
	}

	if next, _ := strconv.ParseBool(q.Get(consts.QUERY_CURSOR_NEXT)); next {
		return types.CursorForward(types.Asc(column), value), nil
	}
	return types.CursorBackward(types.Asc(column), value), nil
}
