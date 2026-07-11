package client

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types/consts"
)

func WithQuery(_keyValues ...any) Option {
	if len(_keyValues) == 0 || len(_keyValues) == 1 {
		return func(_ *Client) {}
	}
	keyValues := make([]string, 0, len(_keyValues))
	for i := range _keyValues {
		val := reflect.ValueOf(_keyValues[i])
		if val.Kind() == reflect.Pointer && !val.IsNil() {
			val = val.Elem()
		}
		switch val.Kind() {
		case reflect.String:
			if str := strings.TrimSpace(val.String()); len(str) > 0 {
				keyValues = append(keyValues, str)
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			keyValues = append(keyValues, fmt.Sprint(val.Interface()))
		case reflect.Float32, reflect.Float64:
			keyValues = append(keyValues, fmt.Sprintf("%.6f", val.Float()))
		case reflect.Bool:
			keyValues = append(keyValues, strconv.FormatBool(val.Bool()))
		}
	}

	length := len(keyValues)
	if length == 0 || length == 1 {
		return func(_ *Client) {}
	}
	if length%2 != 0 {
		length--
	}

	var queryBuilder strings.Builder
	queryBuilder.Grow(length * 8)
	for i := 0; i < length; i += 2 {
		if i > 0 {
			queryBuilder.WriteByte('&')
		}
		key := url.QueryEscape(keyValues[i])
		value := url.QueryEscape(keyValues[i+1])
		queryBuilder.WriteString(key)
		queryBuilder.WriteByte('=')
		queryBuilder.WriteString(value)
	}

	return func(c *Client) {
		c.queryRaw = queryBuilder.String()
	}
}

func WithQueryPagination(page, size int) Option {
	return func(c *Client) {
		if c.query == nil {
			c.query = new(model.Query)
		}
		if page == 0 {
			page = 1
		}
		if size == 0 {
			size = 10
		}
		c.query.Page = page
		c.query.Size = size
	}
}

func WithQueryExpand(expand string, depth uint) Option {
	return func(c *Client) {
		if c.query == nil {
			c.query = new(model.Query)
		}
		if expand = strings.TrimSpace(expand); len(expand) == 0 {
			return
		}
		c.query.Expand = &expand
		c.query.Depth = &depth
	}
}

func WithQueryFuzzy(fuzzy bool) Option {
	return func(c *Client) {
		if c.query == nil {
			c.query = new(model.Query)
		}
		c.query.Fuzzy = &fuzzy
	}
}

func WithQuerySortby(sortby string) Option {
	return func(c *Client) {
		if sortby = strings.TrimSpace(sortby); len(sortby) == 0 {
			return
		}
		if c.query == nil {
			c.query = new(model.Query)
		}
		c.query.SortBy = sortby
	}
}

func WithQueryNocache(nocache bool) Option {
	return func(c *Client) {
		if c.query == nil {
			c.query = new(model.Query)
		}
		c.query.NoCache = nocache
	}
}

func WithQueryTimeRange(columeName string, start, end time.Time) Option {
	return func(c *Client) {
		if c.query == nil {
			c.query = new(model.Query)
		}
		if columeName = strings.TrimSpace(columeName); len(columeName) == 0 {
			return
		}
		if start.IsZero() || end.IsZero() {
			return
		}
		if start.After(end) {
			start, end = end, start
		}
		c.query.ColumnName = columeName
		c.query.StartTime = start.Format(consts.DATE_TIME_LAYOUT)
		c.query.EndTime = end.Format(consts.DATE_TIME_LAYOUT)
	}
}

func WithQueryOr(or bool) Option {
	return func(c *Client) {
		if c.query == nil {
			c.query = new(model.Query)
		}
		c.query.Or = &or
	}
}

func WithQueryIndex(index string) Option {
	return func(c *Client) {
		if index = strings.TrimSpace(index); len(index) == 0 {
			return
		}
		if c.query == nil {
			c.query = new(model.Query)
		}
		c.query.Index = index
	}
}

func WithQuerySelect(selects ...string) Option {
	return func(c *Client) {
		_selects := make([]string, 0, len(selects))
		for i := range selects {
			if len(strings.TrimSpace(selects[i])) != 0 {
				_selects = append(_selects, strings.TrimSpace(selects[i]))
			}
		}
		if len(_selects) == 0 {
			return
		}
		if c.query == nil {
			c.query = new(model.Query)
		}
		c.query.Select = strings.Join(_selects, ",")
	}
}
