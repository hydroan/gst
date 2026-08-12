package client

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-querystring/query"
	"github.com/hydroan/gst/model"
)

// RequestOption populates per-request state such as query parameters. Options
// are per call: the client instance itself stays free of request state.
type RequestOption func(*requestConfig)

// requestConfig carries the per-request state RequestOption can populate.
type requestConfig struct {
	// query holds framework-owned query parameters. Parameter names come from
	// the url tags on model.Query, the single authority for those names.
	query model.Query
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
		c.query.Page = page
		c.query.Size = size
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
// Bounds are formatted as RFC3339, the only layout the framework accepts for
// URL time filtering. A zero time leaves that bound unset; a blank column
// adds nothing.
func WithTimeRange(column string, from, to time.Time) RequestOption {
	return func(c *requestConfig) {
		column = strings.TrimSpace(column)
		if column == "" {
			return
		}
		if !from.IsZero() {
			c.values.Add(column+"[gte]", from.Format(time.RFC3339))
		}
		if !to.IsZero() {
			c.values.Add(column+"[lte]", to.Format(time.RFC3339))
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
