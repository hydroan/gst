package controller

import (
	"reflect"
	"sort"
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
	consts.QUERY_EXPAND:  {},
	consts.QUERY_DEPTH:   {},
	consts.QUERY_SORT_BY: {},
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

// listPageQueryKey is enabled by model.Pagination only: offset paging
// conflicts with cursor semantics, so cursor-only models reject it.
var listPageQueryKey = map[string]struct{}{
	consts.QUERY_PAGE: {},
}

// listSizeQueryKey is enabled by model.Pagination or model.Cursor: both
// paging styles need a client-adjustable page/batch size.
var listSizeQueryKey = map[string]struct{}{
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
// Field-condition keys ("field[op]") are excluded before decoding: they are
// parsed and validated separately by parseFieldConditionsQuery.
func decodeListQuery[M types.Model](m M, query map[string][]string) error {
	query = stripFieldConditionKeys(query)
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
	paginatable := modelregistry.IsPaginatable(m)
	cursorable := modelregistry.IsCursorable(m)
	if !paginatable {
		if err := rejectListQueryKeys(query, listPageQueryKey); err != nil {
			return err
		}
	}
	if !paginatable && !cursorable {
		if err := rejectListQueryKeys(query, listSizeQueryKey); err != nil {
			return err
		}
	}
	if !cursorable {
		if err := rejectListQueryKeys(query, listCursorQueryKeys); err != nil {
			return err
		}
	}
	// A cursor-only model has no struct field carrying the _size tag (it
	// lives in Pagination), so drop the already-validated key before the
	// schema decode; the controller reads _size from the URL directly.
	if cursorable && !paginatable {
		filtered := make(map[string][]string, len(query))
		for key, values := range query {
			if key == consts.QUERY_SIZE {
				continue
			}
			filtered[key] = values
		}
		query = filtered
	}
	return serviceregistry.QueryDecoder().Decode(m, query)
}

// resolveListPagination normalizes the client page/size for a List request.
// sizeAdjustable marks models embedding Pagination or Cursor: their unset
// size defaults to defaultPageSize and oversized values clamp to maxPageSize.
// Models without client size control keep defaultLimit as the full-table
// safety bottom line. An active cursor resets page to 1 so offset paging
// cannot stack on top of cursor filtering.
func resolveListPagination(page, size int, sizeAdjustable, cursorActive bool) (int, int) {
	if sizeAdjustable {
		switch {
		case size <= 0:
			size = defaultPageSize
		case size > maxPageSize:
			size = maxPageSize
		}
	} else if size <= 0 {
		size = defaultLimit
	}
	if cursorActive {
		page = 1
	}
	return page, size
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
		if isFieldConditionKey(key) {
			continue
		}
		if len(strings.Join(values, "")) == 0 {
			continue
		}
		present[strcase.SnakeCase(key)] = struct{}{}
	}
	return present
}

// maxExpandDepth caps the _depth parameter. Every depth level becomes one
// more recursive preload query, so the cap keeps a single request from
// fanning out unbounded database work.
const maxExpandDepth = 10

// isFieldConditionKey reports whether a query key carries a field-level
// operator filter ("field[op]"). Keys in the framework "_" namespace never
// count: an underscore key with brackets stays a framework parameter and is
// rejected by the regular query decoding path.
func isFieldConditionKey(key string) bool {
	return !strings.HasPrefix(key, "_") && strings.ContainsRune(key, '[')
}

// bareTimeFilterColumns are the framework-managed Base/AutoBase timestamp
// columns. They carry query:"-" on the model, so their bare keys are not
// schema-decodable and are handled by parseFieldConditionsQuery instead: the
// bare key is an exact-match (eq) filter, keeping the "bare name filters
// exactly" contract uniform across every documented parameter.
var bareTimeFilterColumns = map[string]struct{}{
	"created_at": {},
	"updated_at": {},
}

// isFieldConditionQueryKey reports whether parseFieldConditionsQuery owns the
// key: either an operator filter key or a bare framework timestamp key.
func isFieldConditionQueryKey(key string) bool {
	if isFieldConditionKey(key) {
		return true
	}
	_, ok := bareTimeFilterColumns[key]
	return ok
}

// stripFieldConditionKeys returns a copy of the query without the keys owned
// by parseFieldConditionsQuery, so gorilla/schema decoding of the model's own
// filter fields never sees them.
func stripFieldConditionKeys(query map[string][]string) map[string][]string {
	filtered := make(map[string][]string, len(query))
	for key, values := range query {
		if isFieldConditionQueryKey(key) {
			continue
		}
		filtered[key] = values
	}
	return filtered
}

// parseFieldConditionsQuery extracts field-level operator filters from URL
// query keys of the form "field[op]=value", e.g. "age[gt]=20" or
// "remark[like]=hello", plus the bare framework timestamp keys
// ("created_at", "updated_at"), which act as exact-match (eq) filters.
// The field token must resolve (after snake case
// normalization) to a queryable column of the model, and op must be a known
// types.FilterOp; anything else is rejected so a mistyped filter can never
// silently widen the result set. Empty values mean "not filtering" and are
// skipped. Field conditions require the model to embed model.Query, and the
// returned conditions are sorted by key for deterministic SQL.
func parseFieldConditionsQuery(m types.Model, query map[string][]string) ([]types.FieldCondition, error) {
	keys := make([]string, 0)
	for key := range query {
		if isFieldConditionQueryKey(key) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	if !modelregistry.IsQueryable(m) {
		sort.Strings(keys)
		return nil, errors.Newf("schema: invalid path %q", keys[0])
	}
	sort.Strings(keys)

	columns := queryableColumns(reflect.TypeOf(m).Elem())
	conds := make([]types.FieldCondition, 0, len(keys))
	for _, key := range keys {
		var field string
		var op types.FilterOp
		if _, bare := bareTimeFilterColumns[key]; bare && !isFieldConditionKey(key) {
			// The bare framework timestamp key is an exact-match filter.
			field, op = key, types.FilterOpEq
		} else {
			var opToken string
			var ok bool
			field, opToken, ok = splitFieldConditionKey(key)
			if !ok {
				return nil, errors.Newf("invalid field filter %q: expect \"field[op]=value\"", key)
			}
			if op, ok = types.ParseFilterOp(opToken); !ok {
				return nil, errors.Newf("invalid field filter %q: unknown operator %q", key, opToken)
			}
		}
		column := strcase.SnakeCase(field)
		columnTyp, ok := columns[column]
		if !ok {
			return nil, errors.Newf("invalid field filter %q: unknown field %q", key, field)
		}
		value := query[key][0]
		if len(value) == 0 {
			continue
		}
		value, err := normalizeFieldConditionValue(columnTyp, op, value)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid field filter %q", key)
		}
		conds = append(conds, types.FieldCondition{Column: column, Op: op, Value: value})
	}
	if len(conds) == 0 {
		return nil, nil
	}
	// Field conditions are always AND-combined, but WithQuery builds the OR
	// chain flat and cannot express (a OR b) AND cond; allowing the mix would
	// let a condition escape the OR group and silently widen the result set,
	// so the combination fails closed instead.
	if values, ok := query[consts.QUERY_OR]; ok && len(values) > 0 {
		if or, err := strconv.ParseBool(values[0]); err == nil && or {
			return nil, errors.Newf("field filters cannot be combined with %s=true", consts.QUERY_OR)
		}
	}
	return conds, nil
}

// splitFieldConditionKey splits "field[op]" into its field and operator
// tokens, reporting whether the key has exactly that shape.
func splitFieldConditionKey(key string) (field, op string, ok bool) {
	open := strings.IndexByte(key, '[')
	if open <= 0 || !strings.HasSuffix(key, "]") {
		return "", "", false
	}
	field, op = key[:open], key[open+1:len(key)-1]
	if len(op) == 0 || strings.ContainsAny(field, "[]") || strings.ContainsAny(op, "[]") {
		return "", "", false
	}
	return field, op, true
}

// fieldConditionTimeLayout is the canonical layout a time-typed field
// condition value is normalized to before it is bound as a statement
// parameter. It preserves any sub-second precision produced by whole-day
// upper-bound extension.
const fieldConditionTimeLayout = "2006-01-02 15:04:05.999999999"

// baseLiftedColumns are the Base/AutoBase struct fields exposed as filterable
// columns, mirroring database.structFieldToMap plus the framework-managed
// timestamps, which are only reachable through operator filters.
var baseLiftedColumns = map[string]string{
	"id":         "ID",
	"created_by": "CreatedBy",
	"updated_by": "UpdatedBy",
	"created_at": "CreatedAt",
	"updated_at": "UpdatedAt",
}

// queryableColumns collects the columns a client can filter on and their Go
// field types, keyed by snake case column name. It mirrors the surface the
// database layer builds conditions from (database.structFieldToMap and
// queryColumnName): the model's own fields with the query tag taking priority
// over the json tag and the field name (a "-" tag opts the field out), nested
// non-framework structs recursively, and the baseLiftedColumns from
// Base/AutoBase.
func queryableColumns(typ reflect.Type, cols ...map[string]reflect.Type) map[string]reflect.Type {
	columns := make(map[string]reflect.Type)
	if len(cols) > 0 {
		columns = cols[0]
	}
	for field := range typ.Fields() {
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		fieldTyp := field.Type
		for fieldTyp.Kind() == reflect.Pointer {
			fieldTyp = fieldTyp.Elem()
		}
		if modelregistry.IsQueryMarkerType(fieldTyp) {
			continue
		}
		switch fieldTyp.Kind() {
		case reflect.Chan, reflect.Map, reflect.Func:
			continue
		case reflect.Struct:
			if field.Name == "Base" || field.Name == "AutoBase" {
				for column, fieldName := range baseLiftedColumns {
					if baseField, ok := fieldTyp.FieldByName(fieldName); ok {
						columns[column] = baseField.Type
					}
				}
				continue
			}
			if fieldTyp != timeType {
				queryableColumns(fieldTyp, columns)
				continue
			}
		}
		column := fieldQueryColumn(field)
		if column == "-" {
			continue
		}
		columns[column] = fieldTyp
	}
	return columns
}

// timeType is the reflect type time-typed columns are recognized by.
var timeType = reflect.TypeFor[time.Time]()

// normalizeFieldConditionValue validates a field condition value against the
// column's Go type and rewrites it into the canonical form bound to the
// statement, so a malformed value is rejected with an error instead of being
// passed to the database where implicit conversion could silently match the
// wrong rows.
//
//   - isnull applies to any column and requires a boolean value, normalized
//     to 1/0; it is handled before the type dispatch below.
//   - time columns accept the comparison operators only; the value is parsed
//     by parseQueryTime and rendered in the server's local zone. A date-only
//     value extends to the end of the day when it forms an upper inclusive
//     (lte) or lower exclusive (gt) bound, so the bound covers the whole day.
//   - bool columns accept eq/ne with a boolean value, normalized to 1/0 to
//     match how exact model filters store bools.
//   - numeric columns require numeric values; in/notin validate every
//     comma-separated member.
//   - string and other columns pass the value through unchanged.
func normalizeFieldConditionValue(columnTyp reflect.Type, op types.FilterOp, value string) (string, error) {
	// isnull is the only operator whose value type is independent of the
	// column type: it always carries a boolean and applies to any nullable
	// column, including time columns the comparison gating below would block.
	if op == types.FilterOpIsNull {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return "", errors.Newf("isnull expects a boolean value, got %q", value)
		}
		if b {
			return "1", nil
		}
		return "0", nil
	}
	switch {
	case columnTyp == timeType:
		switch op {
		case types.FilterOpEq, types.FilterOpNe, types.FilterOpGt, types.FilterOpGte, types.FilterOpLt, types.FilterOpLte:
			end := op == types.FilterOpLte || op == types.FilterOpGt
			t, err := parseQueryTime(value, end)
			if err != nil {
				return "", err
			}
			return t.In(time.Local).Format(fieldConditionTimeLayout), nil
		default:
			return "", errors.Newf("operator %q is not supported on a time field", op)
		}
	case columnTyp.Kind() == reflect.Bool:
		switch op {
		case types.FilterOpEq, types.FilterOpNe:
			b, err := strconv.ParseBool(value)
			if err != nil {
				return "", errors.Newf("expect a boolean value, got %q", value)
			}
			if b {
				return "1", nil
			}
			return "0", nil
		default:
			return "", errors.Newf("operator %q is not supported on a bool field", op)
		}
	case isNumericKind(columnTyp.Kind()):
		switch op {
		case types.FilterOpIn, types.FilterOpNotIn:
			for item := range strings.SplitSeq(value, ",") {
				if err := validateNumericValue(columnTyp.Kind(), item); err != nil {
					return "", err
				}
			}
		case types.FilterOpLike, types.FilterOpNotLike, types.FilterOpStartsWith, types.FilterOpEndsWith:
			// Substring matching relies on the database's string rendering of
			// the number; the pattern itself is not numeric.
		default:
			if err := validateNumericValue(columnTyp.Kind(), value); err != nil {
				return "", err
			}
		}
		return value, nil
	default:
		return value, nil
	}
}

func isNumericKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func validateNumericValue(kind reflect.Kind, value string) error {
	var err error
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, err = strconv.ParseInt(value, 10, 64)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		_, err = strconv.ParseUint(value, 10, 64)
	case reflect.Float32, reflect.Float64:
		_, err = strconv.ParseFloat(value, 64)
	}
	if err != nil {
		return errors.Newf("expect a numeric value, got %q", value)
	}
	return nil
}

// fieldQueryColumn resolves a struct field to its snake case query column,
// with the query tag taking priority over the json tag and the field name;
// it mirrors database.queryColumnName.
func fieldQueryColumn(field reflect.StructField) string {
	name := strings.TrimSpace(field.Tag.Get("query"))
	if idx := strings.IndexByte(name, ','); idx >= 0 {
		name = name[:idx]
	}
	if len(name) == 0 {
		name = strings.TrimSpace(field.Tag.Get("json"))
		if idx := strings.IndexByte(name, ','); idx >= 0 {
			name = name[:idx]
		}
	}
	if len(name) == 0 {
		name = field.Name
	}
	return strcase.SnakeCase(name)
}

// timeQueryLayout describes one accepted layout of a time-typed field
// condition value. dateOnly marks layouts without a time-of-day component so
// an upper bound can extend to the last instant of the day.
type timeQueryLayout struct {
	layout   string
	dateOnly bool
}

// timeQueryLayouts are the zone-less layouts tried in order when parsing a
// time-typed field condition value; they are interpreted in the server's
// local zone. RFC 3339 values with an explicit offset and all-digit unix
// timestamps are handled separately in parseQueryTime.
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

// parseQueryTime parses a time-typed query value. Zone-less layouts are
// interpreted in the server's local zone, RFC 3339 values keep their explicit
// offset, and digit-only values are unix seconds or milliseconds. When end is
// true and the value carries no time of day, the result extends to the last
// instant of that day so an upper bound covers the whole day.
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
// the model's expandable association paths. Expand names are matched against
// m.Expands() ignoring case and snake case punctuation, so "childItems" and
// "child_items" both select "ChildItems"; "_expand=all" selects every
// expandable path. _depth (clamped to [1,10], default 1) repeats slice
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
		if depth < 1 || depth > maxExpandDepth {
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
			if strings.EqualFold(strcase.SnakeCase(item), strcase.SnakeCase(e)) {
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
