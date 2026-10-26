package openapigen

import (
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/urlquery"
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

// addQueryParameters documents the URL query parameters the List or Export
// operation of the framework's own flow takes, those the parsers take (see
// urlquery.Parameters), each described by the doc comment of the field it
// reads. A parameter taking its bare key is documented under it, the
// operators its column takes listed in its description; a column taking its
// operators alone is documented under each of them, the key as the URL spells
// it, expires_at[gte]. An operation that pages its records, a List, takes the
// parameters of model.Pagination and model.Cursor too; an export, which lists
// every record the query matches, refuses them and documents none.
func addQueryParameters[M types.Model, REQ types.Request, RSP types.Response](op *openapi3.Operation, paged bool) {
	// Model-field query filters are available only to the default CRUD path.
	if !modelregistry.AreTypesEqual[M, REQ, RSP]() {
		return
	}

	typ := reflect.TypeFor[M]()
	m := reflect.New(typ.Elem()).Interface().(types.Model) //nolint:errcheck
	params, err := urlquery.Parameters(typ)
	if err != nil {
		// The parsers refuse every parameter of a model gorm cannot parse,
		// which is no model a list reads.
		return
	}
	// The columns of the parameters taking their operators alone, whose
	// values the operator schemas describe.
	columns, _ := modelschema.FilterableIndex(typ)

	queries := make([]*openapi3.ParameterRef, 0, len(params))
	for _, param := range params {
		field, owner := structFieldAt(typ, param.Index)
		if !paged && (owner == reflect.TypeFor[modelregistry.Pagination]() || owner == reflect.TypeFor[modelregistry.Cursor]()) {
			continue
		}
		description := openAPIDocComment(field.Name, modelFieldDocs(reflect.New(owner).Interface())[field.Name])
		if !param.Bare {
			for _, filterOp := range param.Ops {
				queries = append(queries, queryParameter(param.Name+"["+string(filterOp)+"]",
					operatorSchema(columns[param.Name], filterOp), operatorDescription(description, filterOp)))
			}
			continue
		}
		schemaRef := schemaFromType(field.Type)
		if enumDoc, onItems, ok := fieldEnumDoc(field.Type); ok && schemaRef != nil && schemaRef.Value != nil {
			applyEnum(schemaRef.Value, onItems, enumDoc)
			description = enumDescription(description, enumDoc)
		}
		if len(param.Ops) > 0 {
			description = operatorFilterDescription(description, param.Name, param.Ops)
		}
		// The _expand parameter only accepts the model's expandable
		// association names, so list them where the frontend reads them.
		if param.Name == consts.QUERY_EXPAND {
			description = expandableFieldsDescription(description, m.Expands())
		}
		queries = append(queries, queryParameter(param.Name, schemaRef, description))
	}

	// Cursor-only models accept _size as the batch size, but the field with
	// its query tag lives in Pagination, so the parameter is synthesized.
	if paged && modelregistry.IsCursorable(m) && !modelregistry.IsPaginatable(m) {
		queries = append(queries, queryParameter(consts.QUERY_SIZE,
			&openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}},
			"Batch size for cursor pagination."))
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

// queryParameter is an optional query parameter, as every query parameter of
// a list is.
func queryParameter(name string, schemaRef *openapi3.SchemaRef, description string) *openapi3.ParameterRef {
	return &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:        name,
			In:          "query",
			Required:    false,
			Schema:      schemaRef,
			Description: description,
		},
	}
}

// structFieldAt returns the struct field an index path reaches in typ,
// through the pointers on the way, and the struct type declaring it, whose
// doc comments describe it.
func structFieldAt(typ reflect.Type, index []int) (field reflect.StructField, owner reflect.Type) {
	owner = typ
	for depth, i := range index {
		for owner.Kind() == reflect.Pointer {
			owner = owner.Elem()
		}
		field = owner.Field(i)
		if depth < len(index)-1 {
			owner = field.Type
		}
	}
	return field, owner
}

// operatorSchema is the schema of the value of an operator key, field[op], of
// a column no bare key decodes into: isnull takes a boolean, the like family
// and the comma-separated members of in and notin take text, and a
// comparison takes the value the column holds, a time of day as text, an
// instant in RFC 3339, a boolean, a number, or text.
func operatorSchema(col modelschema.Column, filterOp types.FilterOp) *openapi3.SchemaRef {
	switch filterOp {
	case types.FilterOpIsNull:
		return &openapi3.SchemaRef{Value: openapi3.NewBoolSchema()}
	case types.FilterOpEq, types.FilterOpNe, types.FilterOpGt, types.FilterOpGte, types.FilterOpLt, types.FilterOpLte:
		switch {
		case modelschema.IsTimeOfDayColumn(col):
			return &openapi3.SchemaRef{Value: openapi3.NewStringSchema()}
		case modelschema.IsTimeColumn(col):
			return &openapi3.SchemaRef{Value: dateTimeSchema()}
		case modelschema.IsBoolColumn(col):
			return &openapi3.SchemaRef{Value: openapi3.NewBoolSchema()}
		case modelschema.IsNumericColumn(col):
			return &openapi3.SchemaRef{Value: openapi3.NewFloat64Schema()}
		}
	}
	return &openapi3.SchemaRef{Value: openapi3.NewStringSchema()}
}

// operatorDescription is the description of an operator key: the field's
// own, with how the value reads for in and notin, a comma-separated list, and
// for isnull, a boolean.
func operatorDescription(description string, filterOp types.FilterOp) string {
	var note string
	switch filterOp {
	case types.FilterOpIn, types.FilterOpNotIn:
		note = "Comma-separated values."
	case types.FilterOpIsNull:
		note = "true matches a null field, false a field holding a value."
	default:
		return description
	}
	if description == "" {
		return note
	}
	return description + "\n\n" + note
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
// parameter description, listing ops, the operators the "field[op]=value"
// syntax takes on the field's column.
func operatorFilterDescription(description, queryTag string, ops []types.FilterOp) string {
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
