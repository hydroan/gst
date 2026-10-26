package modelschema

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/consts"
	gormschema "gorm.io/gorm/schema"
)

// Column is one database column of a model.
//
// QueryName and DBName are deliberately separate: QueryName is the URL
// parameter clients filter by, named by the field's query tag, and stays a
// front-end contract, while DBName is whatever gorm writes into SQL. A field
// without a query tag has no QueryName; a tagged one usually names the
// parameter as its column, and the two diverge when a model carries a column
// tag or a field name gorm renders differently.
//
// dataType and gormDataType are the data types gorm resolved for the field,
// which tell what the column stores where the Go type does not: the value a
// Valuer such as sql.NullTime holds, or a JSON column a type tag declares.
// They are gorm's vocabulary and stay inside this package: IsTimeColumn,
// IsDateColumn, IsNumericColumn, ValidateNumericValue, IsBoolColumn and
// IsJSONColumn answer from them.
type Column struct {
	GoName       string              // Go selector path of the field: its name, or First.Label inside a struct embedded by name.
	QueryName    string              // URL parameter name: the query tags along the field's path, empty when a step carries none.
	DBName       string              // Database column name resolved by gorm.
	Type         reflect.Type        // Go type of the underlying struct field.
	dataType     gormschema.DataType // Column type gorm resolved: the type tag when the field carries one, else gormDataType.
	gormDataType gormschema.DataType // Data type gorm reads from the Go type, a Valuer through the value it holds.
	Index        []int               // Struct field index path, usable with reflect.Value.FieldByIndex.
	Filterable   bool                // Whether clients may filter on the column through the URL: it has a URL name and is not hidden from JSON.
	NotNull      bool                // Whether the schema forbids NULL: a NOT NULL tag or the primary key.
	PrimaryKey   bool                // Whether the column is part of the model's primary key.
	Creatable    bool                // Whether an insert writes the column: gorm writes none it writes on update only (<-:update) or never.
	Updatable    bool                // Whether an update writes the column: gorm writes none it writes on insert only (<-:create) or never.
}

// schemaCache is the parse cache gorm expects; parsing a model type is only
// done once per process.
var schemaCache = &sync.Map{}

// namer matches the naming strategy the database drivers use. Every driver
// opens gorm with a default gorm.Config, so the zero NamingStrategy is the
// one that runs in production.
var namer = gormschema.NamingStrategy{}

// columnsCache memoizes the resolved columns per model type.
var columnsCache sync.Map

// Columns returns every database column of a model struct type, sorted by
// database column name so repeated calls and generated code stay stable. The
// type may be a struct or a pointer to one.
func Columns(typ reflect.Type) ([]Column, error) {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return nil, errors.Newf("modelschema: expect a struct type, got %v", typ)
	}
	if cached, ok := columnsCache.Load(typ); ok {
		return cached.([]Column), nil //nolint:errcheck
	}

	parsed, err := gormschema.Parse(reflect.New(typ).Interface(), schemaCache, namer)
	if err != nil {
		return nil, errors.Wrapf(err, "modelschema: parse %s", typ.String())
	}

	// FieldsByDBName is gorm's deduplicated column set: a field declared on the
	// model shadows the one lifted from an embedded struct under the same column
	// name, and only the winner appears here. Iterating parsed.Fields instead
	// would yield both and produce two columns with one name.
	columns := make([]Column, 0, len(parsed.FieldsByDBName))
	for _, field := range parsed.FieldsByDBName {
		index, goName, queryName, hidden := fieldPath(typ, field.StructField)
		columns = append(columns, Column{
			GoName:       goName,
			QueryName:    queryName,
			DBName:       field.DBName,
			Type:         field.FieldType,
			dataType:     field.DataType,
			gormDataType: field.GORMDataType,
			Index:        index,
			// A field without a URL name is no client parameter, and one
			// hidden from JSON, framework-managed bookkeeping such as the
			// soft-delete timestamp, stays a real column clients must not
			// filter on, a query tag on it notwithstanding.
			Filterable: queryName != "" && !hidden,
			NotNull:    field.NotNull || field.PrimaryKey,
			PrimaryKey: field.PrimaryKey,
			Creatable:  field.Creatable,
			Updatable:  field.Updatable,
		})
	}
	sort.Slice(columns, func(i, j int) bool { return columns[i].DBName < columns[j].DBName })

	columnsCache.Store(typ, columns)
	return columns, nil
}

// fieldPath reads the path that reaches the field a column reads from the
// model type. index is the path reflect follows, Value.FieldByIndex among
// others, where gorm records a step through an embedded pointer as -i-1. The
// names spell the path: for the Label of a struct the model embeds by the
// name First, both tagged query:"first" and query:"label", the Go name is
// First.Label and the URL name first.label, each step named by its query tag
// (see QueryColumnName); a step without one leaves the field no URL name. An
// anonymous embedded struct adds no step, its fields promoted in Go, so the
// fields of model.Base keep their own names. hidden reports a field kept out
// of JSON, by a json:"-" on the field or on a struct holding it.
//
// gorm keeps the path as the field's bind names, but with every embedded
// struct in it, Base.ID for the ID Go promotes, and with no query tags, so
// the path is walked here, along the field index gorm records.
func fieldPath(model reflect.Type, field reflect.StructField) (index []int, goName, queryName string, hidden bool) {
	hiddenFromJSON := func(f reflect.StructField) bool { return strings.TrimSpace(f.Tag.Get(consts.TAG_JSON)) == "-" }
	index = make([]int, 0, len(field.Index))
	goNames := make([]string, 0, len(field.Index))
	queryNames := make([]string, 0, len(field.Index))
	typ := model
	for _, i := range field.Index[:len(field.Index)-1] {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if i < 0 {
			i = -i - 1
		}
		index = append(index, i)
		step := typ.Field(i)
		hidden = hidden || hiddenFromJSON(step)
		if !step.Anonymous {
			goNames = append(goNames, step.Name)
			queryNames = append(queryNames, QueryColumnName(step))
		}
		typ = step.Type
	}
	index = append(index, field.Index[len(field.Index)-1])
	goNames = append(goNames, field.Name)
	queryNames = append(queryNames, QueryColumnName(field))
	if slices.Contains(queryNames, "") {
		queryNames = nil
	}
	return index, strings.Join(goNames, "."), strings.Join(queryNames, "."), hidden || hiddenFromJSON(field)
}

// QueryColumnName returns the URL parameter name of a struct field, its query
// tag as written up to any options after a comma, or "" when the field
// carries none or query:"-": the json tag serializes a field and never names
// a URL parameter.
//
// This is the client-facing name only. It never decides the database column
// name, which comes from gorm (see Column). Column resolution above, which
// names each step of a field's path by it, the urlquery decode plan and its
// query name check are its consumers, and the OpenAPI query parameters are
// rendered from the list urlquery.Parameters builds on them, which is what
// keeps a field's bare key, its operator-filter key and its documentation on
// one naming rule.
func QueryColumnName(field reflect.StructField) string {
	value := strings.TrimSpace(field.Tag.Get(consts.TAG_QUERY))
	if idx := strings.IndexByte(value, ','); idx >= 0 {
		value = value[:idx]
	}
	if value == "-" {
		return ""
	}
	return strings.TrimSpace(value)
}
