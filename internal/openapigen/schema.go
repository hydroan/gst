package openapigen

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

var (
	timeType = reflect.TypeFor[time.Time]()

	idFormat = "" // eg: "uuid"
)

func getFieldTag(field reflect.StructField, tagName string) string {
	val := field.Tag.Get(tagName)
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
func newSchemaRefWithDocs(value any) *openapi3.SchemaRef {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(value, nil, openapi3gen.CreateTypeNameGenerator(uniqueComponentName))
	if err != nil {
		return schemaRef
	}
	addSchemaDocsForType(reflect.TypeOf(value), schemaRef, nil)
	return schemaRef
}

// addSchemaDocsForType decorates schemaRef with the doc comments and enum
// values registered for typ. Field comments become descriptions without being
// copied into titles, so documentation renderers do not display the same text
// twice and independent schema titles remain intact. It walks the generated
// schema tree and the Go type tree in parallel, so nested request and response
// structs are decorated at every depth. visiting holds the struct types on the
// current descent path so self-referential types terminate; callers pass nil.
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
		jsonTag := getFieldTag(field, consts.TAG_JSON)
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

	gen := openapi3gen.NewGenerator()
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
			jsonTag := getFieldTag(f, consts.TAG_JSON)
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
		return &openapi3.SchemaRef{Value: fieldToOpenAPISchema(reflect.StructField{Type: dataType})}
	}
}

func fieldToOpenAPISchema(field reflect.StructField) *openapi3.Schema {
	typ := field.Type
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	if typ == timeType {
		return dateTimeSchema()
	}

	return &openapi3.Schema{Type: fieldToOpenAPIType(field)}
}

func fieldToOpenAPIType(field reflect.StructField) *openapi3.Types {
	typ := field.Type

	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.String:
		return &openapi3.Types{openapi3.TypeString}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &openapi3.Types{openapi3.TypeInteger}
	case reflect.Float32, reflect.Float64:
		return &openapi3.Types{openapi3.TypeNumber}
	case reflect.Bool:
		return &openapi3.Types{openapi3.TypeBoolean}
	case reflect.Array, reflect.Slice:
		return &openapi3.Types{openapi3.TypeArray}
	case reflect.Struct, reflect.Map:
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
