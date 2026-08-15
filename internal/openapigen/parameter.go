package openapigen

import (
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

func parseParametersFromPath(path string) []*openapi3.ParameterRef {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)

	var params []string
	for _, m := range matches {
		if len(m) > 1 {
			params = append(params, m[1])
		}
	}

	parameterRefList := make([]*openapi3.ParameterRef, 0, len(params))

	for _, param := range params {
		parameterRefList = append(parameterRefList, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				In:       "path",
				Name:     param,
				Required: true,
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:   &openapi3.Types{openapi3.TypeString},
						Format: idFormat,
					},
				},
			},
		})
	}

	return parameterRefList
}

// addQueryParameters adds query parameters for List operation.
func addQueryParameters[M types.Model, REQ types.Request, RSP types.Response](op *openapi3.Operation) {
	// Model-field query filters are available only to the default CRUD path.
	if !modelregistry.AreTypesEqual[M, REQ, RSP]() {
		return
	}

	typ := reflect.TypeFor[M]()
	fields := collectQueryDocFields(typ)
	m := reflect.New(typ.Elem()).Interface().(types.Model) //nolint:errcheck
	queryable := modelregistry.IsQueryable(m)

	queries := make([]*openapi3.ParameterRef, 0, len(fields))
	for _, docField := range fields {
		field := docField.field
		queryTag := getFieldTag(field, consts.TAG_QUERY)
		description := openAPIDocComment(field.Name, docField.docs[field.Name])
		schemaRef := schemaFromType(field.Type)
		if enumDoc, onItems, ok := fieldEnumDoc(field.Type); ok && schemaRef != nil && schemaRef.Value != nil {
			applyEnum(schemaRef.Value, onItems, enumDoc)
			description = enumDescription(description, enumDoc)
		}
		// Business filter fields on queryable models additionally accept the
		// "field[op]=value" operator filter syntax; framework parameters in
		// the "_" namespace do not.
		if queryable && !strings.HasPrefix(queryTag, "_") {
			description = operatorFilterDescription(description, queryTag)
		}
		// The _expand parameter only accepts the model's expandable
		// association names, so list them where the frontend reads them.
		if queryable && queryTag == consts.QUERY_EXPAND {
			description = expandableFieldsDescription(description, m.Expands())
		}

		queries = append(queries, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        queryTag,
				In:          "query",
				Required:    false,
				Schema:      schemaRef,
				Description: description,
			},
		})
	}

	// Cursor-only models accept _size as the batch size, but the field with
	// its query tag lives in Pagination, so the parameter is synthesized.
	if modelregistry.IsCursorable(m) && !modelregistry.IsPaginatable(m) {
		queries = append(queries, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        consts.QUERY_SIZE,
				In:          "query",
				Required:    false,
				Schema:      &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}},
				Description: "Batch size for cursor pagination.",
			},
		})
	}

	// The framework-managed Base/AutoBase timestamps carry query:"-" on the
	// model, so their filter parameters are synthesized: the bare name is an
	// exact-match filter like every other documented parameter, and the
	// operator syntax covers ranges. The columns are declared as an ordered
	// slice, not a map, because sortQueryParameters is stable and keeps their
	// insertion order in the generated document.
	if queryable && embedsBaseModel(typ.Elem()) {
		for _, timestamp := range []struct{ column, doc string }{
			{"created_at", "record creation time"},
			{"updated_at", "record last update time"},
		} {
			column, doc := timestamp.column, timestamp.doc
			queries = append(queries, &openapi3.ParameterRef{
				Value: &openapi3.Parameter{
					Name:        column,
					In:          "query",
					Required:    false,
					Schema:      &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
					Description: "Exact-match filter for the " + doc + ".\n\nOperator filter: " + column + "[op]=value, op: eq/ne/gt/gte/lt/lte/isnull; range example: " + column + "[gte]=2026-07-01&" + column + "[lte]=2026-07-15.",
				},
			})
		}
	}

	// Business filter columns always come first; framework parameters keep a
	// canonical trailing order regardless of where the framework structs are
	// embedded in the model.
	sortQueryParameters(queries)

	// Avoid duplicate additions
	existing := map[string]bool{}
	for _, p := range op.Parameters {
		if p.Value != nil {
			existing[p.Value.Name] = true
		}
	}

	for _, query := range queries {
		if query.Value != nil && !existing[query.Value.Name] {
			op.Parameters = append(op.Parameters, query)
			existing[query.Value.Name] = true
		}
	}
}

// collectQueryDocFields collects query-tagged fields from typ and anonymous
// embedded structs in breadth-first order. Fields declared at a shallower
// depth win over deeper promoted fields with the same query name, matching Go
// field selection precedence.
func collectQueryDocFields(typ reflect.Type) []schemaDocField {
	if typ == nil {
		return nil
	}

	fields := make([]schemaDocField, 0)
	seen := make(map[string]bool)
	visited := make(map[reflect.Type]bool)
	queue := []reflect.Type{typ}
	for len(queue) > 0 {
		next := make([]reflect.Type, 0)
		for _, currentType := range queue {
			for currentType.Kind() == reflect.Pointer {
				currentType = currentType.Elem()
			}
			if currentType.Kind() != reflect.Struct || visited[currentType] {
				continue
			}
			visited[currentType] = true

			docs := modelFieldDocs(reflect.New(currentType).Interface())
			for field := range currentType.Fields() {
				queryTag := getFieldTag(field, consts.TAG_QUERY)
				if queryTag != "" {
					if !seen[queryTag] {
						seen[queryTag] = true
						fields = append(fields, schemaDocField{field: field, docs: docs})
					}
					continue
				}
				if !field.Anonymous {
					continue
				}

				embeddedType := field.Type
				for embeddedType.Kind() == reflect.Pointer {
					embeddedType = embeddedType.Elem()
				}
				if embeddedType.Kind() == reflect.Struct {
					next = append(next, embeddedType)
				}
			}
		}
		queue = next
	}
	return fields
}

// embedsBaseModel reports whether the model struct embeds Base or AutoBase.
func embedsBaseModel(typ reflect.Type) bool {
	if typ.Kind() != reflect.Struct {
		return false
	}
	for _, name := range []string{"Base", "AutoBase"} {
		if field, ok := typ.FieldByName(name); ok && field.Anonymous {
			return true
		}
	}
	return false
}

// frameworkQueryParameterOrder is the canonical trailing order of framework
// query parameters in generated API documents, sorted by how commonly each
// parameter is used: pagination on every list page, then table sorting, then
// cursor pagination (the primary paging style for large datasets), and finally
// association expansion, which is meaningful only on models that declare
// expandable associations.
var frameworkQueryParameterOrder = []string{
	consts.QUERY_PAGE, consts.QUERY_SIZE, consts.QUERY_SORT_BY,
	consts.QUERY_CURSOR_VALUE, consts.QUERY_CURSOR_FIELD, consts.QUERY_CURSOR_NEXT,
	consts.QUERY_EXPAND, consts.QUERY_DEPTH,
}

// sortQueryParameters puts business filter parameters first, preserving their
// collection order, and framework "_" parameters after them in the canonical
// frameworkQueryParameterOrder; framework parameters missing from the
// canonical list keep their relative order at the end.
func sortQueryParameters(queries []*openapi3.ParameterRef) {
	rank := func(name string) int {
		if !strings.HasPrefix(name, "_") {
			return 0
		}
		for i, known := range frameworkQueryParameterOrder {
			if name == known {
				return 1 + i
			}
		}
		return 1 + len(frameworkQueryParameterOrder)
	}
	sort.SliceStable(queries, func(i, j int) bool {
		return rank(queries[i].Value.Name) < rank(queries[j].Value.Name)
	})
}

// expandableFieldsDescription appends the model's expandable association
// names to the _expand parameter description so the frontend can see the
// accepted values.
func expandableFieldsDescription(description string, expands []string) string {
	var note string
	if len(expands) == 0 {
		note = "This model has no expandable associations."
	} else {
		note = "Expandable: " + strings.Join(expands, ", ") + ", or all (snake case accepted, matched case-insensitively)."
	}
	if len(description) == 0 {
		return note
	}
	return description + "\n\n" + note
}

// operatorFilterDescription appends the field operator filter note to a query
// parameter description, listing the operators accepted by the
// "field[op]=value" syntax.
func operatorFilterDescription(description, queryTag string) string {
	ops := types.FilterOps()
	tokens := make([]string, 0, len(ops))
	for _, op := range ops {
		tokens = append(tokens, string(op))
	}
	note := "Operator filter: " + queryTag + "[op]=value, op: " + strings.Join(tokens, "/") + "."
	if len(description) == 0 {
		return note
	}
	return description + "\n\n" + note
}
