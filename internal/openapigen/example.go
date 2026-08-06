package openapigen

import "github.com/getkin/kin-openapi/openapi3"

// applyExample builds the request body example and removes the Base auto
// fields ("created_at", "created_by", "updated_at", "updated_by", "id") from
// the top level only. Nested structs keep those property names because there
// they are caller-supplied fields rather than Base auto fields.
//
// Before:
//
//	{
//	  "created_at": "2025-04-19T19:22:55.434Z",
//	  "created_by": "string",
//	  "desc": "string",
//	  "id": "string",
//	  "member_count": 0,
//	  "name": "string",
//	  "updated_at": "2025-04-19T19:22:55.434Z",
//	  "updated_by": "string"
//	}
//
// After:
//
//	{
//	  "desc": "string",
//	  "member_count": 0,
//	  "name": "string"
//	}
//
// NOTE: struct fields must carry a json tag, otherwise they are missing from schemaRef.Value.Properties.
func applyExample(schemaRef *openapi3.SchemaRef) {
	if schemaRef == nil {
		return
	}
	if schemaRef.Value == nil {
		schemaRef.Value = new(openapi3.Schema)
	}

	examples := make(map[string]any)
	for k, v := range schemaRef.Value.Properties {
		if baseAutoFieldNames[k] {
			continue
		}
		if v.Value == nil {
			continue
		}
		examples[k] = buildExampleValue(v.Value, 0)
	}
	schemaRef.Value.Example = examples
}

// applyBatchExample builds the example of a batch request: one example for a
// single element of the items array, and the whole batch example around it.
func applyBatchExample(schemaRef *openapi3.SchemaRef) {
	if schemaRef == nil || schemaRef.Value == nil {
		return
	}

	props := schemaRef.Value.Properties
	for k, v := range props {
		if k == "items" && v.Value != nil && v.Value.Type.Is(openapi3.TypeArray) {
			if v.Value.Items != nil && v.Value.Items.Value != nil {
				// build the example for a single element of the array
				example := make(map[string]any)
				for propName, propRef := range v.Value.Items.Value.Properties {
					if baseAutoFieldNames[propName] || propRef.Value == nil {
						continue
					}
					example[propName] = buildExampleValue(propRef.Value, 0)
				}

				// set the example of a single item
				v.Value.Items.Value.Example = example

				// set the example of the whole batch request
				schemaRef.Value.Example = map[string]any{
					"items": []map[string]any{example},
				}
			}
		}
	}
}

// maxExampleDepth bounds buildExampleValue recursion so a self-referential
// type (eg. a tree or linked-list struct) can't recurse indefinitely.
const maxExampleDepth = 10

// buildExampleValue recursively builds an example value for schema so nested
// arrays, structs, and maps (additionalProperties) show their full shape in
// Swagger instead of an empty placeholder.
//
// A self-referential type recurses until maxExampleDepth stops it. The descent
// stops with an empty array or object rather than with a null, because the
// member being filled is typically not nullable and a null there makes the
// example fail validation against the very schema it illustrates.
func buildExampleValue(schema *openapi3.Schema, depth int) any {
	if schema == nil {
		return nil
	}

	// An enum accepts nothing but its declared values, whatever its JSON type.
	if len(schema.Enum) > 0 {
		return schema.Enum[0]
	}

	// A schema without a type accepts any value, eg. the value side of a
	// user-defined JSON map. Only an explicitly nullable member may stay null,
	// so illustrate the rest with a string.
	if schema.Type == nil {
		if schema.Nullable {
			return nil
		}
		return "string"
	}

	switch {
	case schema.Type.Is(openapi3.TypeString):
		if example, ok := exampleForStringFormat(schema.Format); ok {
			return example
		}
		return "string"
	case schema.Type.Is(openapi3.TypeInteger):
		return 0
	case schema.Type.Is(openapi3.TypeNumber):
		return 0.0
	case schema.Type.Is(openapi3.TypeBoolean):
		return false
	case schema.Type.Is(openapi3.TypeArray):
		if isRefOrMissing(schema.Items) || depth >= maxExampleDepth {
			return []any{}
		}
		return []any{buildExampleValue(schema.Items.Value, depth+1)}
	case schema.Type.Is(openapi3.TypeObject):
		if depth >= maxExampleDepth {
			return map[string]any{}
		}
		if len(schema.Properties) > 0 {
			example := make(map[string]any, len(schema.Properties))
			for propName, propRef := range schema.Properties {
				if isRefOrMissing(propRef) {
					continue
				}
				// Nested fields keep their id/audit-named properties: at this
				// depth they are caller-supplied fields, not the Base auto
				// fields that only appear at the request top level.
				example[propName] = buildExampleValue(propRef.Value, depth+1)
			}
			return example
		}
		if !isRefOrMissing(schema.AdditionalProperties.Schema) {
			return map[string]any{"string": buildExampleValue(schema.AdditionalProperties.Schema.Value, depth+1)}
		}
		return map[string]any{}
	default:
		return nil
	}
}

// isRefOrMissing reports whether schemaRef carries no usable inline schema to
// build an example from.
//
// A member rendered as a $ref keeps an inline Value in memory, but that Value
// is dropped on serialization and, being reached through a cycle, never got
// decorated with the enum values and formats its component carries. Building an
// example from it invents values the referenced component rejects, so the
// descent stops at the boundary and readers follow the $ref instead.
func isRefOrMissing(schemaRef *openapi3.SchemaRef) bool {
	return schemaRef == nil || schemaRef.Ref != "" || schemaRef.Value == nil
}

// exampleForStringFormat returns the example value of a formatted string. A
// bare "string" placeholder is rejected by validators for these formats, since
// the format carries its own pattern.
func exampleForStringFormat(format string) (string, bool) {
	switch format {
	case "date-time":
		return "2006-01-02T15:04:05Z", true
	case "date":
		return "2006-01-02", true
	default:
		return "", false
	}
}
