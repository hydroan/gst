package urlquery

import (
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/consts"
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
	value := q.Get(consts.QUERY_CURSOR_VALUE)

	// An unnamed column leaves the order column empty on purpose: the database
	// layer owns the primary key fallback, so both this path and a service
	// building a cursor by hand land on the same default.
	column := ""
	var columnType reflect.Type
	if field := strings.TrimSpace(q.Get(consts.QUERY_CURSOR_FIELD)); len(field) > 0 {
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
		columnType = resolved.Type
	} else {
		// Value validation still resolves the fallback column the database
		// layer will compare against. The lookup goes by database column name
		// over the full schema rather than the filterable set, because the
		// fallback is framework-owned, not client-named. A model without the
		// column keeps a nil type and the value passes through unvalidated:
		// such a cursor only means something to a service that queries a
		// different model with it.
		parsed, err := modelschema.Columns(reflect.TypeOf(m))
		if err != nil {
			return types.Cursor{}, err
		}
		for _, col := range parsed {
			if col.DBName == modelregistry.DefaultCursorColumn {
				columnType = col.Type
				break
			}
		}
	}
	if len(value) == 0 {
		// The first page: the feed from its start, in its order, with no
		// boundary to start past.
		return types.CursorForward(types.Asc(column), ""), nil
	}
	value, err := normalizeCursorValue(columnType, value)
	if err != nil {
		return types.Cursor{}, err
	}

	if next, _ := strconv.ParseBool(q.Get(consts.QUERY_CURSOR_NEXT)); next {
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
	if len(q.Get(consts.QUERY_CURSOR_VALUE)) > 0 {
		return true
	}
	if len(q.Get(consts.QUERY_SORT_BY)) > 0 {
		return false
	}
	return q.Has(consts.QUERY_CURSOR_FIELD) || q.Has(consts.QUERY_CURSOR_NEXT) || !modelregistry.IsPaginatable(m)
}

// normalizeCursorValue checks the boundary value against the Go type of the
// cursor column and returns the value the SQL comparison binds, mirroring the
// fail-closed filter semantics: a mistyped "field[op]=value" filter is
// rejected, so a mistyped cursor value must not fare better. Without the
// check the raw string reaches the SQL comparison, where MySQL coerces it
// instead of failing — a non-numeric boundary on a numeric column becomes 0 —
// and the feed silently restarts from the first page.
//
// A time boundary is not only validated but normalized exactly like a time
// filter bound (see timeBound): the client spells the boundary in RFC 3339,
// and the database compares the one wall clock the framework stores on every
// dialect, the UTC day at midnight for a date column.
//
// Only types the database coerces lossily are gated: numeric, bool and time
// columns, a pointer column checked as the type it points to. String and
// other column types accept any value, and a nil column type (the model
// cannot resolve the fallback column, see Cursor) keeps the plain
// passthrough.
func normalizeCursorValue(columnTyp reflect.Type, value string) (string, error) {
	if columnTyp == nil {
		return value, nil
	}
	for columnTyp.Kind() == reflect.Pointer {
		columnTyp = columnTyp.Elem()
	}
	var err error
	switch class := modelschema.ClassifyColumn(columnTyp); {
	case class == modelschema.ColumnClassTime:
		var bound string
		if bound, err = timeBound(columnTyp, value); err == nil {
			value = bound
		}
	case columnTyp.Kind() == reflect.Bool:
		if _, parseErr := strconv.ParseBool(value); parseErr != nil {
			err = errors.Newf("expect a boolean value, got %q", value)
		}
	case class == modelschema.ColumnClassNumeric:
		err = validateNumericValue(columnTyp.Kind(), value)
	}
	if err != nil {
		return "", errors.Wrapf(err, "invalid cursor value")
	}
	return value, nil
}
