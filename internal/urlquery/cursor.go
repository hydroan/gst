package urlquery

import (
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// Cursor returns the cursor position of the request, ready to be passed to
// Database.WithCursor.
//
// A model opts in to cursor pagination by embedding model.Cursor; any other
// model yields a zero cursor, which WithCursor treats as a no-op. The cursor
// column is validated against the model's filterable columns and the cursor
// value against that column's Go type, so an unknown column or a mistyped
// value fails here instead of reaching the database, which would coerce the
// value instead of failing (MySQL turns a non-numeric boundary on a numeric
// column into 0) and silently restart the feed from the first page.
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
	var columnType reflect.Type
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
	if err := validateCursorValue(columnType, value); err != nil {
		return types.Cursor{}, err
	}

	if next, _ := strconv.ParseBool(q.Get(consts.QUERY_CURSOR_NEXT)); next {
		return types.CursorForward(types.Asc(column), value), nil
	}
	return types.CursorBackward(types.Asc(column), value), nil
}

// validateCursorValue checks the boundary value against the Go type of the
// cursor column, mirroring the fail-closed filter semantics: a mistyped
// "field[op]=value" filter is rejected, so a mistyped cursor value must not
// fare better. Without the check the raw string reaches the SQL comparison,
// where MySQL coerces it instead of failing — a non-numeric boundary on a
// numeric column becomes 0 — and the feed silently restarts from the first
// page.
//
// Only types the database coerces lossily are gated: numeric, bool and time
// columns. String and other column types accept any value, and a nil column
// type (the model cannot resolve the fallback column, see Cursor) keeps the
// plain passthrough.
func validateCursorValue(columnTyp reflect.Type, value string) error {
	if columnTyp == nil {
		return nil
	}
	var err error
	switch {
	case columnTyp == timeType:
		_, err = parseQueryTime(value, false)
	case columnTyp.Kind() == reflect.Bool:
		if _, parseErr := strconv.ParseBool(value); parseErr != nil {
			err = errors.Newf("expect a boolean value, got %q", value)
		}
	case isNumericKind(columnTyp.Kind()):
		err = validateNumericValue(columnTyp.Kind(), value)
	}
	return errors.Wrapf(err, "invalid cursor value")
}
