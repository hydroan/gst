package controller

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stoewer/go-strcase"
)

// listQueryKeys are List controller parameters that belong to model.Query
// rather than to the resource model's own filter fields.
var listQueryKeys = map[string]struct{}{
	consts.QUERY_EXPAND:      {},
	consts.QUERY_DEPTH:       {},
	consts.QUERY_FUZZY:       {},
	consts.QUERY_SORT_BY:     {},
	consts.QUERY_TIME_COLUMN: {},
	consts.QUERY_START_TIME:  {},
	consts.QUERY_END_TIME:    {},
}

// listUnsafeQueryKeys are enabled by model.UnsafeQuery. They are split from
// listQueryKeys because they rewrite filter combination or tune query
// execution; in particular _or can defeat mandatory service-level filters,
// so a model must opt in to them separately from the regular List controls.
var listUnsafeQueryKeys = map[string]struct{}{
	consts.QUERY_OR:       {},
	consts.QUERY_INDEX:    {},
	consts.QUERY_SELECT:   {},
	consts.QUERY_NO_CACHE: {},
	consts.QUERY_NO_TOTAL: {},
}

// listPaginationQueryKeys are enabled by model.Pagination. They are split from
// listQueryKeys so a model can allow page and size without enabling fuzzy,
// sorting, expansion, or other framework-owned List controls.
var listPaginationQueryKeys = map[string]struct{}{
	consts.QUERY_PAGE: {},
	consts.QUERY_SIZE: {},
}

// listCursorQueryKeys are enabled by model.Cursor. Cursor pagination is
// intentionally independent from SortBy; the cursor field and direction define
// the stable order used by the database layer.
var listCursorQueryKeys = map[string]struct{}{
	consts.QUERY_CURSOR_VALUE: {},
	consts.QUERY_CURSOR_FIELD: {},
	consts.QUERY_CURSOR_NEXT:  {},
}

// decodeListQuery decodes URL query parameters into the model's filter fields,
// rejecting framework query keys the model has not opted in to via
// model.Query, model.UnsafeQuery, model.Pagination, or model.Cursor.
func decodeListQuery[M types.Model](m M, query map[string][]string) error {
	if !modelregistry.IsQueryable(m) {
		if err := rejectListQueryKeys(query, listQueryKeys); err != nil {
			return err
		}
	}
	if !modelregistry.IsUnsafeQueryable(m) {
		if err := rejectListQueryKeys(query, listUnsafeQueryKeys); err != nil {
			return err
		}
	}
	if !modelregistry.IsPaginatable(m) {
		if err := rejectListQueryKeys(query, listPaginationQueryKeys); err != nil {
			return err
		}
	}
	if !modelregistry.IsCursorable(m) {
		if err := rejectListQueryKeys(query, listCursorQueryKeys); err != nil {
			return err
		}
	}
	return serviceregistry.QueryDecoder().Decode(m, query)
}

func rejectListQueryKeys(query map[string][]string, keys map[string]struct{}) error {
	for key := range query {
		if _, found := keys[key]; found {
			return errors.Newf("schema: invalid path %q", key)
		}
	}
	return nil
}

// presentQueryFields collects the model filter keys explicitly provided in the
// URL query string, keyed by snake case column name, so the database layer can
// keep zero values (false, 0) of these columns as query conditions. Framework
// parameters (the "_" prefix namespace) and keys whose values are all empty
// are excluded: they are not model filter columns, and an empty value means
// the caller is not filtering by that key.
func presentQueryFields(query map[string][]string) map[string]struct{} {
	present := make(map[string]struct{}, len(query))
	for key, values := range query {
		if strings.HasPrefix(key, "_") {
			continue
		}
		if len(strings.Join(values, "")) == 0 {
			continue
		}
		present[strcase.SnakeCase(key)] = struct{}{}
	}
	return present
}

// timeQueryLayout describes one accepted layout of the _start_time and
// _end_time query parameters. dateOnly marks layouts without a time-of-day
// component so an end bound can extend to the last instant of the day.
type timeQueryLayout struct {
	layout   string
	dateOnly bool
}

// timeQueryLayouts are the zone-less layouts tried in order when parsing
// _start_time/_end_time; they are interpreted in the server's local zone.
// RFC 3339 values with an explicit offset and all-digit unix timestamps are
// handled separately in parseQueryTime.
var timeQueryLayouts = []timeQueryLayout{
	{layout: consts.DATE_TIME_LAYOUT}, // 2006-01-02 15:04:05
	{layout: "2006-01-02T15:04:05"},   // HTML datetime-local with seconds
	{layout: "2006-01-02 15:04"},
	{layout: "2006-01-02T15:04"}, // HTML datetime-local
	{layout: "2006-01-02", dateOnly: true},
}

// unixMilliThreshold separates unix-second from unix-millisecond values:
// digit-only values at or above it (13+ digits) are treated as milliseconds.
const unixMilliThreshold = 1e12

// parseTimeRangeQuery parses the _start_time/_end_time query parameters.
// Absent or empty values yield zero times, which disable the corresponding
// bound in WithTimeRange. Unparseable values are reported as errors instead
// of being silently dropped, so a malformed bound cannot silently widen the
// result set.
func parseTimeRangeQuery(c *gin.Context) (startTime, endTime time.Time, err error) {
	if value, ok := c.GetQuery(consts.QUERY_START_TIME); ok && len(value) > 0 {
		if startTime, err = parseQueryTime(value, false); err != nil {
			return startTime, endTime, errors.Wrapf(err, "invalid %s", consts.QUERY_START_TIME)
		}
	}
	if value, ok := c.GetQuery(consts.QUERY_END_TIME); ok && len(value) > 0 {
		if endTime, err = parseQueryTime(value, true); err != nil {
			return startTime, endTime, errors.Wrapf(err, "invalid %s", consts.QUERY_END_TIME)
		}
	}
	return startTime, endTime, nil
}

// parseQueryTime parses a single time bound from a query parameter value.
// Zone-less layouts are interpreted in the server's local zone, RFC 3339
// values keep their explicit offset, and digit-only values are unix seconds
// or milliseconds. When end is true and the value carries no time of day,
// the result extends to the last instant of that day so an inclusive upper
// bound covers the whole day.
func parseQueryTime(value string, end bool) (time.Time, error) {
	for _, l := range timeQueryLayouts {
		t, err := time.ParseInLocation(l.layout, value, time.Local)
		if err != nil {
			continue
		}
		if end && l.dateOnly {
			t = t.AddDate(0, 0, 1).Add(-time.Nanosecond)
		}
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, nil
	}
	if isDigitsOnly(value) {
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			if n >= unixMilliThreshold {
				return time.UnixMilli(n), nil
			}
			return time.Unix(n, 0), nil
		}
	}
	return time.Time{}, errors.Newf("unsupported time format %q", value)
}

func isDigitsOnly(value string) bool {
	if len(value) == 0 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseExpandQuery resolves the _expand and _depth query parameters against
// the model's expandable association paths. Expand names are matched
// case-insensitively against m.Expands(), and "_expand=all" selects every
// expandable path. _depth (clamped to [1,99], default 1) repeats slice
// associations for recursive preloading, e.g. expand "Children" with depth 3
// becomes "Children.Children.Children"; non-slice associations ignore depth.
func parseExpandQuery(c *gin.Context, m types.Model) []string {
	expandStr, ok := c.GetQuery(consts.QUERY_EXPAND)
	if !ok {
		return nil
	}
	depth := 1
	if depthStr, ok := c.GetQuery(consts.QUERY_DEPTH); ok {
		depth, _ = strconv.Atoi(depthStr)
		if depth < 1 || depth > 99 {
			depth = 1
		}
	}

	items := strings.Split(expandStr, ",")
	if len(items) > 0 && items[0] == consts.VALUE_ALL { // expand all fields
		items = m.Expands()
	}
	var matched []string
	for _, e := range m.Expands() {
		for _, item := range items {
			if strings.EqualFold(item, e) {
				matched = append(matched, e)
			}
		}
	}

	typ := reflect.TypeOf(m).Elem()
	fieldsMap := make(map[string]reflect.Kind)
	for field := range typ.Fields() {
		fieldsMap[field.Name] = field.Type.Kind()
	}
	var expands []string
	for _, e := range matched {
		// If the expanding field does not exist in the structure fields, skip depth expand.
		kind, found := fieldsMap[e]
		if !found {
			expands = append(expands, e)
			continue
		}
		// If the expanding field exists in the structure but the kind is not slice, skip depth expand.
		if kind != reflect.Slice {
			expands = append(expands, e)
			continue
		}
		t := make([]string, depth)
		for i := range depth {
			t[i] = e
		}
		// If expand="Children" and depth=3, the depth expanded is "Children.Children.Children".
		expands = append(expands, strings.Join(t, "."))
	}
	return expands
}
