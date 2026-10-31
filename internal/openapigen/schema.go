package openapigen

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/internal/modelschema"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	timeType          = reflect.TypeFor[time.Time]()
	jsonMapType       = reflect.TypeFor[datatypes.JSONMap]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()

	idFormat = "" // eg: "uuid"
)

// bindingTag is the struct tag gin validates a request by.
const bindingTag = "binding"

// jsonName returns the name a struct field's json tag gives it, or "" for a
// field the tag leaves unnamed or out (json:"-").
func jsonName(field reflect.StructField) string {
	val := field.Tag.Get(modelschema.TagJSON)
	switch val {
	case "-", "":
		return ""
	default:
		return strings.Split(val, ",")[0]
	}
}

// newSchemaRefWithDocs generates the OpenAPI schema for value and decorates it
// and every nested schema with the doc comments and enum values registered for
// the Go types reachable from value's type.
//
// A self-referential type, eg. a tree node holding its own children, cannot be
// inlined forever, so the generator breaks the cycle by emitting a $ref into
// components. Those $ref targets are named through uniqueComponentName, the
// same rule registerSchema names components with, otherwise the $ref points at
// a component that was never registered and the whole document fails to load.
//
// The zero value of an interface-typed payload or result is nil: such a type
// binds any JSON value, which the empty schema describes.
//
// A type whose encoding methods decide its JSON is documented as what they
// write (see documentEncodingMethods).
//
// A fixed-length array type, [N]T, is documented with the empty schema as
// well, as though it held any value: openapi3gen describes slices but not
// arrays. A slice type documents the element type.
func newSchemaRefWithDocs(value any) *openapi3.SchemaRef {
	if value == nil {
		return openapi3.NewSchemaRef("", openapi3.NewSchema())
	}
	schemaRef, err := openapi3gen.NewSchemaRefForValue(value, nil, openapi3gen.CreateTypeNameGenerator(uniqueComponentName), openapi3gen.SchemaCustomizer(documentEncodingMethods))
	if err != nil {
		return schemaRef
	}
	addSchemaDocsForType(reflect.TypeOf(value), schemaRef, nil)
	return schemaRef
}

// jsonWrite is what encoding/json writes for a type with encoding methods
// of its own, as a schema describes it: a nil typ is the empty schema, any
// JSON value, and nullable marks a type that writes null for a zero value.
type jsonWrite struct {
	typ      *openapi3.Types
	format   string
	nullable bool
}

// knownJSONWrites are the types with encoding methods of their own whose
// JSON the framework knows, the ones the generators know as well (see
// jsonshape.BuiltinOf): a datatypes.Date writes the RFC 3339 string
// time.Time writes, a datatypes.Time a clock string, a gorm.DeletedAt that
// RFC 3339 string or null, a json.Number a number, and a raw JSON document
// (json.RawMessage, datatypes.JSON) any value. time.Time, datatypes.JSONMap
// and datatypes.JSONType are left to openapi3gen, which documents the first
// two as a date-time string and an object, the last unwrapped afterwards
// (see convertDatatypesJSONTypeSchema).
var knownJSONWrites = map[reflect.Type]jsonWrite{
	reflect.TypeFor[datatypes.Date]():  {typ: &openapi3.Types{openapi3.TypeString}, format: "date-time"},
	reflect.TypeFor[datatypes.Time]():  {typ: &openapi3.Types{openapi3.TypeString}},
	reflect.TypeFor[gorm.DeletedAt]():  {typ: &openapi3.Types{openapi3.TypeString}, format: "date-time", nullable: true},
	reflect.TypeFor[json.Number]():     {typ: &openapi3.Types{openapi3.TypeNumber}},
	reflect.TypeFor[json.RawMessage](): {},
	reflect.TypeFor[datatypes.JSON]():  {},
}

// documentEncodingMethods is the openapi3gen customizer documenting a type
// with encoding methods of its own as what encoding/json writes for it (see
// jsonWriteOf), in place of what openapi3gen reflects from its structure: a
// netip.Addr is a string, not an empty object, a json.Number a number, not
// a string, and a type with a MarshalJSON of its own any value, the empty
// schema, nothing telling what the method writes. A pointer to one stays
// nullable.
func documentEncodingMethods(_ string, t reflect.Type, _ reflect.StructTag, schema *openapi3.Schema) error {
	write, ok := jsonWriteOf(t)
	if !ok {
		return nil
	}
	*schema = openapi3.Schema{Type: write.typ, Format: write.format, Nullable: schema.Nullable || write.nullable}
	return nil
}

// jsonWriteOf returns what encoding/json writes for t when its encoding
// methods decide it: a known type's write (see knownJSONWrites), any value
// for another type with a MarshalJSON, which takes precedence over
// MarshalText, and a string for a type with a MarshalText alone, the method
// on t or on its pointer type, as encoding/json finds it. It reports false
// for a type openapi3gen describes by its structure, time.Time,
// datatypes.JSONMap and datatypes.JSONType among them.
func jsonWriteOf(t reflect.Type) (jsonWrite, bool) {
	if write, ok := knownJSONWrites[t]; ok {
		return write, true
	}
	if t == timeType || t == jsonMapType {
		return jsonWrite{}, false
	}
	if _, isJSONType := datatypesJSONDataType(t); isJSONType {
		return jsonWrite{}, false
	}
	implements := func(iface reflect.Type) bool {
		return t.Implements(iface) || reflect.PointerTo(t).Implements(iface)
	}
	switch {
	case implements(jsonMarshalerType):
		return jsonWrite{}, true
	case implements(textMarshalerType):
		return jsonWrite{typ: &openapi3.Types{openapi3.TypeString}}, true
	}
	return jsonWrite{}, false
}

// addSchemaDocsForType decorates schemaRef with the doc comments and enum
// values registered for typ, and marks required the properties whose field's
// own binding rules hold required, binding:"required,max=8" among them, as a
// request without one is refused; binding:"dive,required" requires the
// elements, not the field. Field comments become descriptions without
// being copied into titles, so documentation renderers do not display the
// same text twice and independent schema titles remain intact. It walks the
// generated schema tree and the Go type tree in parallel, so nested request
// and response structs are decorated at every depth. visiting holds the
// struct types on the current descent path so self-referential types
// terminate; callers pass nil.
func addSchemaDocsForType(typ reflect.Type, schemaRef *openapi3.SchemaRef, visiting map[reflect.Type]bool) {
	if typ == nil || schemaRef == nil || schemaRef.Value == nil {
		return
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.Slice, reflect.Array:
		addSchemaDocsForType(typ.Elem(), schemaRef.Value.Items, visiting)
		return
	case reflect.Struct:
	default:
		return
	}
	if typ == timeType || len(schemaRef.Value.Properties) == 0 {
		return
	}
	if visiting[typ] {
		return
	}
	if visiting == nil {
		visiting = make(map[reflect.Type]bool)
	}
	visiting[typ] = true
	defer delete(visiting, typ)

	fields := make(map[string]schemaDocField)
	collectSchemaDocFields(typ, fields, make(map[reflect.Type]bool))

	for propName, propRef := range schemaRef.Value.Properties {
		if propRef == nil || propRef.Value == nil {
			continue
		}
		docField, hasField := fields[propName]
		if !hasField {
			continue
		}
		// The rules before the first dive are the field's own; those after
		// it are the elements' of a slice or a map.
		rules := strings.Split(docField.field.Tag.Get(bindingTag), ",")
		if dive := slices.Index(rules, "dive"); dive >= 0 {
			rules = rules[:dive]
		}
		if slices.Contains(rules, "required") && !slices.Contains(schemaRef.Value.Required, propName) {
			schemaRef.Value.Required = append(schemaRef.Value.Required, propName)
		}

		description := openAPIDocComment(docField.field.Name, docField.docs[docField.field.Name])

		// Unwrap gorm datatypes.JSONType[T] so both the schema and the type
		// walk below continue with the wrapped data type.
		fieldType := docField.field.Type
		if dataType, isJSONType := datatypesJSONDataType(fieldType); isJSONType {
			if unwrapped := convertDatatypesJSONTypeSchema(propRef, docField.field); unwrapped != nil {
				propRef = unwrapped
				schemaRef.Value.Properties[propName] = propRef
			}
			fieldType = dataType
		}

		enumDoc, enumOnItems, hasEnum := fieldEnumDoc(fieldType)
		if (description != "" || hasEnum) && propRef.Value != nil {
			// Copy the schema so shared schema instances keep their own docs.
			newSchema := *propRef.Value
			newSchema.Description = description
			if hasEnum {
				applyEnum(&newSchema, enumOnItems, enumDoc)
				newSchema.Description = enumDescription(description, enumDoc)
			}
			propRef = &openapi3.SchemaRef{Value: &newSchema}
			schemaRef.Value.Properties[propName] = propRef
		}

		addSchemaDocsForType(fieldType, propRef, visiting)
	}
	// The properties come in map order; the document lists the required ones
	// in a stable one.
	slices.Sort(schemaRef.Value.Required)
}

// schemaDocField pairs one JSON-visible struct field with the doc comments of
// the struct that declares it.
type schemaDocField struct {
	field reflect.StructField
	docs  map[string]string
}

// collectSchemaDocFields maps every JSON property name of typ to its declaring
// struct field and that struct's doc comments, descending into anonymous
// embedded structs the same way encoding/json promotes fields. Fields already
// collected win over deeper promoted fields, matching encoding/json
// shallow-first precedence. visited stops self-embedding chains.
func collectSchemaDocFields(typ reflect.Type, fields map[string]schemaDocField, visited map[reflect.Type]bool) {
	if visited[typ] {
		return
	}
	visited[typ] = true

	docs := modelFieldDocs(reflect.New(typ).Interface())

	var embedded []reflect.Type
	for field := range typ.Fields() {
		jsonTag := jsonName(field)
		if field.Anonymous && jsonTag == "" {
			embeddedType := field.Type
			for embeddedType.Kind() == reflect.Pointer {
				embeddedType = embeddedType.Elem()
			}
			if embeddedType.Kind() == reflect.Struct {
				embedded = append(embedded, embeddedType)
			}
			continue
		}
		if jsonTag == "" {
			continue
		}
		if _, exists := fields[jsonTag]; exists {
			continue
		}
		fields[jsonTag] = schemaDocField{field: field, docs: docs}
	}

	for _, embeddedType := range embedded {
		collectSchemaDocFields(embeddedType, fields, visited)
	}
}

// fieldEnumDoc resolves the registered enum doc of a field type, unwrapping
// pointers, slices and arrays. The second result reports whether the enum
// applies to the slice items schema instead of the field schema itself.
func fieldEnumDoc(typ reflect.Type) (doc apidoc.EnumDoc, onItems bool, ok bool) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
		onItems = true
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
	}
	if typ.PkgPath() == "" || typ.Name() == "" {
		return apidoc.EnumDoc{}, false, false
	}
	doc, ok = apidoc.LookupEnum(typ.PkgPath(), typ.Name())
	if !ok || len(doc.Values) == 0 {
		return apidoc.EnumDoc{}, false, false
	}
	doc.Comment = openAPIDocComment(typ.Name(), doc.Comment)
	return doc, onItems, true
}

// applyEnum sets the enum values on the property schema, or on a copy of its
// items schema for slice fields to avoid mutating shared item schemas.
func applyEnum(schema *openapi3.Schema, onItems bool, doc apidoc.EnumDoc) {
	values := make([]any, 0, len(doc.Values))
	for _, value := range doc.Values {
		values = append(values, value.Value)
	}

	if !onItems {
		schema.Enum = values
		return
	}
	if schema.Items == nil || schema.Items.Value == nil {
		return
	}
	items := *schema.Items.Value
	items.Enum = values
	schema.Items = &openapi3.SchemaRef{Value: &items}
}

// enumDescription appends the enum value list to the field description so
// each value's comment stays visible next to the field. When the field has
// no comment of its own, the enum type comment is used as the base text.
func enumDescription(base string, doc apidoc.EnumDoc) string {
	if base == "" {
		base = doc.Comment
	}

	lines := make([]string, 0, len(doc.Values))
	for _, value := range doc.Values {
		line := fmt.Sprintf("- `%v`", value.Value)
		if value.Comment != "" {
			line += ": " + value.Comment
		}
		lines = append(lines, line)
	}
	list := strings.Join(lines, "\n")

	if base == "" {
		return list
	}
	return base + "\n\n" + list
}

// datatypesJSONDataType returns the wrapped data type of a gorm
// datatypes.JSONType[T] field type, unwrapping pointers first.
func datatypesJSONDataType(typ reflect.Type) (reflect.Type, bool) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.PkgPath() != "gorm.io/datatypes" || (typ.Name() != "JSONType" && !strings.HasPrefix(typ.Name(), "JSONType[")) {
		return nil, false
	}
	for f := range typ.Fields() {
		if f.Name == "Data" || f.Name == "data" || f.IsExported() {
			return f.Type, true
		}
	}
	return nil, false
}

// convertDatatypesJSONTypeSchema unwraps gorm datatypes.JSONType[T] so the
// generated schema uses the underlying T definition instead of the wrapper.
func convertDatatypesJSONTypeSchema(propRef *openapi3.SchemaRef, field reflect.StructField) *openapi3.SchemaRef {
	if propRef == nil {
		return nil
	}
	dataType, isJSONType := datatypesJSONDataType(field.Type)
	if !isJSONType {
		return propRef
	}

	value := reflect.Zero(dataType).Interface()

	gen := openapi3gen.NewGenerator(openapi3gen.SchemaCustomizer(documentEncodingMethods))
	schemaRef, err := gen.NewSchemaRefForValue(value, nil)
	if err != nil || schemaRef == nil || schemaRef.Value == nil || (schemaRef.Value.Type == nil && len(schemaRef.Value.Properties) == 0) {
		schemaRef = schemaFromType(dataType)
		if schemaRef == nil {
			zap.S().Warnf("failed to build schema for datatypes.JSONType[%s]: %v", dataType.String(), err)
			return propRef
		}
	}

	return schemaRef
}

func schemaFromType(dataType reflect.Type) *openapi3.SchemaRef {
	return schemaFromTypeVisiting(dataType, nil)
}

// schemaFromTypeVisiting implements schemaFromType. visiting holds the struct
// types on the current descent path so a self-referential type, eg. a tree node
// holding its own children, terminates instead of recursing forever; callers
// pass nil.
func schemaFromTypeVisiting(dataType reflect.Type, visiting map[reflect.Type]bool) *openapi3.SchemaRef {
	for dataType.Kind() == reflect.Pointer {
		dataType = dataType.Elem()
	}

	if dataType == timeType {
		return &openapi3.SchemaRef{Value: dateTimeSchema()}
	}

	switch dataType.Kind() {
	case reflect.Struct:
		schema := openapi3.NewObjectSchema()
		if visiting[dataType] {
			// Reaching the same struct again closes a cycle: describe it as a
			// bare object rather than descend into it once more.
			return &openapi3.SchemaRef{Value: schema}
		}
		if visiting == nil {
			visiting = make(map[reflect.Type]bool)
		}
		visiting[dataType] = true
		defer delete(visiting, dataType)

		for f := range dataType.Fields() {
			if !f.IsExported() {
				continue
			}
			jsonTag := jsonName(f)
			if jsonTag == "" {
				continue
			}
			schema.WithPropertyRef(jsonTag, schemaFromTypeVisiting(f.Type, visiting))
		}
		return &openapi3.SchemaRef{Value: schema}
	case reflect.Slice, reflect.Array:
		itemRef := schemaFromTypeVisiting(dataType.Elem(), visiting)
		if itemRef == nil {
			return nil
		}
		arraySchema := openapi3.NewArraySchema()
		arraySchema.Items = itemRef
		return &openapi3.SchemaRef{Value: arraySchema}
	default:
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: openAPIType(dataType)}}
	}
}

// openAPIType returns the OpenAPI type of a value of the Go type typ, a
// scalar or a map: schemaFromTypeVisiting answers a pointer, a time, a
// struct, a slice and an array itself.
func openAPIType(typ reflect.Type) *openapi3.Types {
	switch typ.Kind() {
	case reflect.String:
		return &openapi3.Types{openapi3.TypeString}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &openapi3.Types{openapi3.TypeInteger}
	case reflect.Float32, reflect.Float64:
		return &openapi3.Types{openapi3.TypeNumber}
	case reflect.Bool:
		return &openapi3.Types{openapi3.TypeBoolean}
	case reflect.Map:
		return &openapi3.Types{openapi3.TypeObject}
	default:
		// An unmapped kind, eg. an interface, constrains nothing. Leaving the
		// type out says exactly that, whereas "null" is not a type OpenAPI 3.0
		// defines and makes the enclosing schema invalid.
		return nil
	}
}

func dateTimeSchema() *openapi3.Schema {
	return &openapi3.Schema{
		Type:   &openapi3.Types{openapi3.TypeString},
		Format: "date-time",
	}
}
