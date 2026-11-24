package client

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-querystring/query"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/types"
)

// RequestOption populates per-request state such as query parameters. Options
// are per call: the client instance itself stays free of request state.
type RequestOption func(*requestConfig)

// requestConfig carries the per-request state RequestOption can populate.
type requestConfig struct {
	// query holds the framework-owned query parameters a model declares.
	// Their names come from the url tags on model.Query and
	// model.Cursor, the single authority for those names.
	query struct {
		model.Query
		model.Cursor
	}
	// page and size are the paging parameters every list takes, which no
	// model declares; zero leaves one out.
	page, size int
	// values holds free-form business filter parameters set by WithQuery.
	values url.Values
}

func newRequestConfig(opts []RequestOption) *requestConfig {
	cfg := &requestConfig{values: url.Values{}}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

// encode renders every populated parameter as one URL query string.
func (c *requestConfig) encode() (string, error) {
	vals, err := query.Values(&c.query)
	if err != nil {
		return "", err
	}
	if c.page != 0 {
		vals.Set(types.QueryPage, strconv.Itoa(c.page))
	}
	if c.size != 0 {
		vals.Set(types.QuerySize, strconv.Itoa(c.size))
	}
	for key, list := range c.values {
		for _, value := range list {
			vals.Add(key, value)
		}
	}
	return vals.Encode(), nil
}

// WithQuery adds free-form query parameters from alternating key/value pairs.
// Values may be strings, integers, floats or booleans; a trailing key without
// a value is dropped. Business filters, including the "field[op]=value"
// operator syntax, go through here.
func WithQuery(keyValues ...any) RequestOption {
	return func(c *requestConfig) {
		for i := 0; i+1 < len(keyValues); i += 2 {
			key := strings.TrimSpace(stringifyQueryValue(keyValues[i]))
			if key == "" {
				continue
			}
			c.values.Add(key, stringifyQueryValue(keyValues[i+1]))
		}
	}
}

// WithPage sets the framework offset pagination parameters (_page, _size).
func WithPage(page, size int) RequestOption {
	return func(c *requestConfig) {
		c.page = page
		c.size = size
	}
}

// WithSortBy sets the framework sorting parameter (_sort_by).
func WithSortBy(sortBy string) RequestOption {
	return func(c *requestConfig) {
		c.query.SortBy = sortBy
	}
}

// WithExpand sets the framework association expansion parameters (_expand, _depth).
func WithExpand(expand string, depth uint) RequestOption {
	return func(c *requestConfig) {
		if expand = strings.TrimSpace(expand); expand == "" {
			return
		}
		c.query.Expand = &expand
		c.query.Depth = &depth
	}
}

// WithCursor sets the framework cursor pagination parameters
// (_cursor_field, _cursor_value, _cursor_next).
func WithCursor(field, value string, next bool) RequestOption {
	return func(c *requestConfig) {
		c.query.CursorField = field
		c.query.CursorValue = &value
		c.query.CursorNext = next
	}
}

// WithTimeRange adds the framework's time-window filter on column, encoded
// in the "field[op]=value" operator syntax as column[gte] and column[lte].
// Bounds are formatted as RFC 3339 with the fractional seconds they carry
// (time.RFC3339Nano), the only layout the framework accepts for URL time
// filtering, so a bound reaches the server as exact as it is given. A zero
// time leaves that bound unset; a blank column adds nothing.
func WithTimeRange(column string, from, to time.Time) RequestOption {
	return func(c *requestConfig) {
		column = strings.TrimSpace(column)
		if column == "" {
			return
		}
		if !from.IsZero() {
			c.values.Add(column+"[gte]", from.Format(time.RFC3339Nano))
		}
		if !to.IsZero() {
			c.values.Add(column+"[lte]", to.Format(time.RFC3339Nano))
		}
	}
}

// stringifyQueryValue renders one query key or value as text. Unsupported
// kinds render empty and the surrounding logic skips them as keys.
func stringifyQueryValue(v any) string {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return ""
		}
		val = val.Elem()
	}
	switch val.Kind() {
	case reflect.String:
		return val.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprint(val.Interface())
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(val.Float(), 'f', -1, 64)
	case reflect.Bool:
		return strconv.FormatBool(val.Bool())
	default:
		return ""
	}
}
