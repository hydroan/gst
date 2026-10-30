package urlquery

import (
	"net/url"
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
)

// Cursor returns the cursor position of the request, ready to be passed to
// Database.WithCursor.
//
// A model opts in to cursor pagination by embedding model.Cursor; any other
// model yields a zero cursor, which WithCursor treats as a no-op, and so does
// a request that does not read the feed through a cursor (see
// cursorRequested): a plain request on a model that also pages by offset, or
// one sorted by the client. A request with a cursor parameter but no
// _cursor_value is the feed's first page: a cursor with the column and no
// boundary, so the first page comes back in the feed's order and the row a
// client pages on from is the row the order puts last.
//
// The cursor column is validated against the model's filterable columns, on
// the first page as on any other, and the cursor value against that
// column's Go type, so an unknown column or a mistyped value fails here
// instead of reaching the database, which would coerce the value instead of
// failing (MySQL turns a non-numeric boundary on a numeric column into 0)
// and silently restart the feed from the first page.
//
// The column must also be one rows cannot share — the primary key, or a NOT
// NULL column with a unique index of its own: a cursor is a single boundary
// value, so on a shared value the rows a page had no room for fall between
// the pages and are never read. A client naming any other column is refused
// rather than served a feed with holes in it.
//
// A URL cursor always pages an ascending feed: _cursor_next only chooses
// whether the request travels along the feed or back down it. A descending
// feed is a service-side cursor, built with types.CursorForward on a Desc
// order.
func Cursor(q url.Values, m types.Model) (types.Cursor, error) {
	if !cursorRequested(q, m) {
		return types.Cursor{}, nil
	}
	value := q.Get(types.QueryCursorValue)

	// An unnamed column leaves the order column empty on purpose: the database
	// layer owns the primary key fallback, so both this path and a service
	// building a cursor by hand land on the same default.
	column := ""
	var cursorColumn modelschema.Column
	if field := strings.TrimSpace(q.Get(types.QueryCursorField)); len(field) > 0 {
		columns, err := modelschema.FilterableIndex(reflect.TypeOf(m))
		if err != nil {
			return types.Cursor{}, err
		}
		resolved, ok := columns[field]
		if !ok {
			return types.Cursor{}, errors.Newf("unknown cursor column %q", field)
		}
		identifying, err := modelschema.IdentifyingColumns(m)
		if err != nil {
			return types.Cursor{}, err
		}
		if _, ok := identifying[resolved.DBName]; !ok {
			// Rows sharing the boundary's value are split between pages, and
			// the ones the page had no room for are never read again. The
			// client is told instead of quietly getting fewer rows.
			return types.Cursor{}, errors.Newf(
				"cursor column %q is not unique: page by the primary key, or by a NOT NULL column with a unique index of its own", field)
		}
		column = resolved.DBName
		cursorColumn = resolved
	} else {
		// Value validation still resolves the fallback column the database
		// layer will compare against. The lookup goes by database column name
		// over the full schema rather than the filterable set, because the
		// fallback is framework-owned, not client-named. A model without the
		// column keeps the zero Column and the value passes through
		// unvalidated: such a cursor only means something to a service that
		// queries a different model with it.
		parsed, err := modelschema.Columns(reflect.TypeOf(m))
		if err != nil {
			return types.Cursor{}, err
		}
		for _, col := range parsed {
			if col.DBName == modelschema.DefaultCursorColumn {
				cursorColumn = col
				break
			}
		}
	}
	if len(value) == 0 {
		// The first page: the feed from its start, in its order, with no
		// boundary to start past.
		return types.CursorForward(types.Asc(column), ""), nil
	}
	value, err := normalizeCursorValue(cursorColumn, value)
	if err != nil {
		return types.Cursor{}, err
	}

	if next, _ := modelschema.ParseBool(q.Get(types.QueryCursorNext)); next {
		return types.CursorForward(types.Asc(column), value), nil
	}
	return types.CursorBackward(types.Asc(column), value), nil
}

// cursorRequested reports whether the request reads the feed of m through a
// cursor. It does when the request carries a boundary value, or a cursor
// parameter without one, which asks for the feed's first page; a model that
// pages by cursor alone has nothing but the feed, so a request naming no
// cursor parameter reads its first page too. A request sorted by the client
// is a list, not the feed, unless it carries a boundary value, which the
// list controller refuses beside _sort_by. Pagination reads the same answer,
// so offset paging never stacks on top of a cursor.
func cursorRequested(q url.Values, m types.Model) bool {
	if !modelregistry.IsCursorable(m) {
		return false
	}
	if len(q.Get(types.QueryCursorValue)) > 0 {
		return true
	}
	if len(q.Get(types.QuerySortBy)) > 0 {
		return false
	}
	return q.Has(types.QueryCursorField) || q.Has(types.QueryCursorNext) || !modelregistry.IsPaginatable(m)
}

// normalizeCursorValue checks the boundary value against what the cursor
// column stores and returns the value the SQL comparison binds, mirroring the
// fail-closed filter semantics: a mistyped "field[op]=value" filter is
// rejected, so a mistyped cursor value must not fare better. Without the
// check the raw string reaches the SQL comparison, where MySQL coerces it
// instead of failing — a non-numeric boundary on a numeric column becomes 0 —
// and the feed silently restarts from the first page.
//
// A time boundary is not only validated but normalized exactly like a time
// filter bound (see timeBound): the client spells the boundary in RFC 3339,
// and the database compares the one wall clock the framework stores on every
// dialect, a date column by the UTC day the boundary falls on; a time of day
// boundary is spelled in time.TimeOnly and binds as datatypes.Time stores it.
//
// Only types the database coerces lossily are gated: numeric, bool, time and
// time of day columns, read from the data types gorm resolved for the column,
// so a pointer or a Valuer such as sql.NullInt64 is checked as the value it
// holds. String and other column types accept any value, and the zero Column
// (the model cannot resolve the fallback column, see Cursor) keeps the plain
// passthrough.
func normalizeCursorValue(col modelschema.Column, value string) (string, error) {
	var err error
	switch {
	case modelschema.IsTimeColumn(col), modelschema.IsTimeOfDayColumn(col):
		var bound string
		if bound, err = timeBound(col, value); err == nil {
			value = bound
		}
	case modelschema.IsBoolColumn(col):
		_, err = modelschema.ParseBool(value)
	case modelschema.IsNumericColumn(col):
		err = modelschema.ValidateNumericValue(col, value)
	}
	if err != nil {
		return "", errors.Wrapf(err, "invalid cursor value")
	}
	return value, nil
}
