// Package modelschema resolves what a model struct declares about its table:
// its database columns, the secondary indexes it declares (see Index), its
// optimistic-locking column (see Version) and the columns identifying one of
// its rows (see IdentifyingColumns).
//
// It is the single place where a Go struct field is mapped to a database
// column: the mapping is delegated to gorm's own schema parser, so column
// names always match what gorm actually emits, including the column tag,
// the ignore markers, embedded struct lifting, and gorm's commonInitialisms
// handling (which a plain snake case conversion does not reproduce). The
// facts built on the columns read them through the same parser.
package modelschema

import (
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/stoewer/go-strcase"
	gormschema "gorm.io/gorm/schema"
)

// Column is one database column of a model.
//
// QueryName and DBName are deliberately separate: QueryName is the URL
// parameter clients filter by and stays a front-end contract, while DBName is
// whatever gorm writes into SQL. They are usually equal, and diverge when a
// model carries a column tag or a field name gorm renders differently.
//
// dataType and gormDataType are the data types gorm resolved for the field,
// which tell what the column stores where the Go type does not: the value a
// Valuer such as sql.NullTime holds, or a JSON column a type tag declares.
// They are gorm's vocabulary and stay inside this package: IsTimeColumn,
// IsDateColumn, IsNumericColumn, ValidateNumericValue, IsBoolColumn and
// IsJSONColumn answer from them.
type Column struct {
	GoName       string              // Go selector path of the field: its name, or First.Label inside a struct embedded by name.
	QueryName    string              // URL parameter name: query tag, else json tag, else field name, in snake case, per step of the JSON path.
	DBName       string              // Database column name resolved by gorm.
	Type         reflect.Type        // Go type of the underlying struct field.
	dataType     gormschema.DataType // Column type gorm resolved: the type tag when the field carries one, else gormDataType.
	gormDataType gormschema.DataType // Data type gorm reads from the Go type, a Valuer through the value it holds.
	Index        []int               // Struct field index path, usable with reflect.Value.FieldByIndex.
	Filterable   bool                // Whether clients may filter on the column through the URL.
	NotNull      bool                // Whether the schema forbids NULL: a NOT NULL tag or the primary key.
	PrimaryKey   bool                // Whether the column is part of the model's primary key.
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
			// A field hidden from JSON is framework-managed bookkeeping such
			// as the soft-delete timestamp: it stays a real column, but
			// clients must not filter on it.
			Filterable: !hidden,
			NotNull:    field.NotNull || field.PrimaryKey,
			PrimaryKey: field.PrimaryKey,
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
// name First, under the json name first, the Go name is First.Label and the
// URL name first.label, each step named by its own rule (see
// QueryColumnName). An anonymous embedded struct adds no step, its fields
// promoted in Go, so the fields of model.Base keep their own names. hidden
// reports a field kept out of JSON, by a json:"-" on the field or on a
// struct holding it.
//
// gorm keeps the path as the field's bind names, but with every embedded
// struct in it, Base.ID for the ID Go promotes, and with no JSON spelling, so
// the path is walked here, along the field index gorm records.
func fieldPath(model reflect.Type, field reflect.StructField) (index []int, goName, queryName string, hidden bool) {
	hiddenFromJSON := func(f reflect.StructField) bool { return strings.TrimSpace(f.Tag.Get("json")) == "-" }
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
	return index, strings.Join(goNames, "."), strings.Join(queryNames, "."), hidden || hiddenFromJSON(field)
}

// QueryColumnName resolves the URL parameter name of a struct field: the
// query tag wins over the json tag, which wins over the field name, and the
// result is converted to snake case. A "-" tag is skipped rather than used
// as a name, so the next source decides.
//
// This is the client-facing name only. It never decides the database column
// name, which comes from gorm (see Column). Column resolution above, which
// names each step of a field's path by it, and the urlquery decode plan are
// its two consumers, which is what keeps a field's bare key and its
// operator-filter key on one naming rule.
func QueryColumnName(field reflect.StructField) string {
	name := tagName(field, "query")
	if name == "" {
		name = tagName(field, "json")
	}
	if name == "" {
		name = field.Name
	}
	return strcase.SnakeCase(name)
}

// tagName reads a struct tag and strips its options suffix.
func tagName(field reflect.StructField, key string) string {
	value := strings.TrimSpace(field.Tag.Get(key))
	if idx := strings.IndexByte(value, ','); idx >= 0 {
		value = value[:idx]
	}
	if value == "-" {
		return ""
	}
	return strings.TrimSpace(value)
}
