package openapigen

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelregistry"
)

func TestAddQueryParametersUsesDateTimeFormatForTimeFields(t *testing.T) {
	op := &openapi3.Operation{}

	addQueryParameters[*openapiTimeQueryModel, *openapiTimeQueryModel, *openapiTimeQueryModel](op)

	for _, parameter := range op.Parameters {
		if parameter.Value == nil || parameter.Value.Name != "expires_at" {
			continue
		}

		assertDateTimeSchema(t, parameter.Value.Schema)
		if parameter.Value.Description != "The expiration time." {
			t.Fatalf("expires_at description = %q, want API-facing field comment", parameter.Value.Description)
		}
		return
	}

	t.Fatal("expires_at query parameter was not added")
}

type openapiPaginationQueryModel struct {
	modelregistry.Pagination
	modelregistry.Base
}

type openapiCursorQueryModel struct {
	modelregistry.Cursor
	modelregistry.Base
}

type openapiDeepQueryFields struct {
	Shared int `json:"-" query:"shared"`
}

type openapiFirstQueryBranch struct {
	openapiDeepQueryFields
}

type openapiSecondQueryBranch struct {
	Shared string `json:"-" query:"shared"`
}

type openapiShallowQueryModel struct {
	openapiFirstQueryBranch
	openapiSecondQueryBranch
	modelregistry.Base
}

func TestAddQueryParametersIncludesEmbeddedFrameworkParameters(t *testing.T) {
	tests := []struct {
		name       string
		add        func(*openapi3.Operation)
		parameters []string
	}{
		{
			name: "query",
			add: func(op *openapi3.Operation) {
				addQueryParameters[*openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel](op)
			},
			parameters: []string{
				"page",
				"_page", "_size",
				"_cursor_value", "_cursor_field", "_cursor_next",
				"_expand", "_depth", "_sort_by",
				"id", "created_by", "updated_by",
				"created_at", "updated_at",
			},
		},
		{
			name: "pagination",
			add: func(op *openapi3.Operation) {
				addQueryParameters[*openapiPaginationQueryModel, *openapiPaginationQueryModel, *openapiPaginationQueryModel](op)
			},
			parameters: []string{"_page", "_size", "id", "created_by", "updated_by"},
		},
		{
			name: "cursor",
			add: func(op *openapi3.Operation) {
				addQueryParameters[*openapiCursorQueryModel, *openapiCursorQueryModel, *openapiCursorQueryModel](op)
			},
			parameters: []string{"_size", "_cursor_value", "_cursor_field", "_cursor_next", "id", "created_by", "updated_by"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &openapi3.Operation{}
			tt.add(op)

			parameters := queryParametersByName(t, op)
			for _, name := range tt.parameters {
				if parameters[name] == nil {
					t.Errorf("query parameter %q is missing", name)
				}
			}
			if len(parameters) != len(tt.parameters) {
				t.Fatalf("query parameters = %v, want exactly %v", parameterNames(parameters), tt.parameters)
			}
		})
	}

	op := &openapi3.Operation{}
	addQueryParameters[*openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel](op)
	page := queryParametersByName(t, op)["page"]
	if page.Schema == nil || page.Schema.Value == nil || page.Schema.Value.Type == nil || !page.Schema.Value.Type.Is(openapi3.TypeString) {
		t.Fatalf("page schema = %#v, want the bare business field to keep its own string schema alongside the framework _page parameter", page.Schema)
	}

	op = &openapi3.Operation{}
	addQueryParameters[*openapiShallowQueryModel, *openapiShallowQueryModel, *openapiShallowQueryModel](op)
	shared := queryParametersByName(t, op)["shared"]
	if shared.Schema == nil || shared.Schema.Value == nil || shared.Schema.Value.Type == nil || !shared.Schema.Value.Type.Is(openapi3.TypeString) {
		t.Fatalf("shared schema = %#v, want the shallower embedded string field to override the earlier deeper field", shared.Schema)
	}
}

func TestAddQueryParametersOrdersBusinessFieldsBeforeFrameworkParameters(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParameters[*openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel](op)

	names := make([]string, 0, len(op.Parameters))
	for _, parameterRef := range op.Parameters {
		names = append(names, parameterRef.Value.Name)
	}
	want := []string{
		"page",
		"id", "created_by", "updated_by",
		"created_at", "updated_at",
		"_page", "_size", "_sort_by",
		"_cursor_value", "_cursor_field", "_cursor_next",
		"_expand", "_depth",
	}
	if len(names) != len(want) {
		t.Fatalf("query parameters = %v, want exactly %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("query parameter order = %v, want %v: business filter columns must come first regardless of where the framework structs are embedded", names, want)
		}
	}
}

type openapiExpandableQueryModel struct {
	Children []*openapiExpandableQueryModel `json:"children,omitempty"`
	Parent   *openapiExpandableQueryModel   `json:"parent,omitempty"`

	modelregistry.Query
	modelregistry.Base
}

func (*openapiExpandableQueryModel) Expands() []string { return []string{"Children", "Parent"} }

func TestAddQueryParametersDocumentsExpandableFields(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParameters[*openapiExpandableQueryModel, *openapiExpandableQueryModel, *openapiExpandableQueryModel](op)
	parameters := queryParametersByName(t, op)

	expand := parameters["_expand"]
	if expand == nil {
		t.Fatal("_expand query parameter was not added")
	}
	for _, token := range []string{"Children", "Parent", "all"} {
		if !strings.Contains(expand.Description, token) {
			t.Fatalf("_expand description = %q, want expandable field %q listed", expand.Description, token)
		}
	}
}

func TestAddQueryParametersDocumentsFieldOperatorFilters(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParameters[*openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel](op)
	parameters := queryParametersByName(t, op)

	if !strings.Contains(parameters["page"].Description, "page[op]=value") {
		t.Fatalf("page description = %q, want a field operator filter note on queryable models", parameters["page"].Description)
	}
	if !strings.Contains(parameters["page"].Description, "notlike") {
		t.Fatalf("page description = %q, want the known operators listed", parameters["page"].Description)
	}
	if strings.Contains(parameters["_sort_by"].Description, "[op]=value") {
		t.Fatalf("_sort_by description = %q, framework parameters must not advertise operator filters", parameters["_sort_by"].Description)
	}

	op = &openapi3.Operation{}
	addQueryParameters[*openapiPaginationQueryModel, *openapiPaginationQueryModel, *openapiPaginationQueryModel](op)
	parameters = queryParametersByName(t, op)
	if strings.Contains(parameters["id"].Description, "[op]=value") {
		t.Fatalf("id description = %q, models without modelregistry.Query must not advertise operator filters", parameters["id"].Description)
	}
}

type openapiSliceQueryModel struct {
	Values []string `json:"-" query:"values"`

	modelregistry.Base
}

func TestAddQueryParametersBuildsSliceItemSchema(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParameters[*openapiSliceQueryModel, *openapiSliceQueryModel, *openapiSliceQueryModel](op)

	values := queryParametersByName(t, op)["values"]
	if values == nil || values.Schema == nil || values.Schema.Value == nil {
		t.Fatal("values query parameter schema is missing")
	}
	if values.Schema.Value.Type == nil || !values.Schema.Value.Type.Is(openapi3.TypeArray) {
		t.Fatalf("values schema type = %v, want array", values.Schema.Value.Type)
	}
	if values.Schema.Value.Items == nil || values.Schema.Value.Items.Value == nil || values.Schema.Value.Items.Value.Type == nil || !values.Schema.Value.Items.Value.Type.Is(openapi3.TypeString) {
		t.Fatalf("values item schema = %#v, want string", values.Schema.Value.Items)
	}
}

type enumQueryModel struct {
	Status enumFieldStatus `json:"status" query:"status"`

	modelregistry.Base
}

func TestAddQueryParametersSetsEnumValues(t *testing.T) {
	registerEnumFieldStatus()

	op := &openapi3.Operation{}
	addQueryParameters[*enumQueryModel, *enumQueryModel, *enumQueryModel](op)

	for _, parameter := range op.Parameters {
		if parameter.Value == nil || parameter.Value.Name != "status" {
			continue
		}
		if len(parameter.Value.Schema.Value.Enum) != 2 {
			t.Fatalf("status parameter enum = %#v, want the two enum values", parameter.Value.Schema.Value.Enum)
		}
		if !strings.Contains(parameter.Value.Description, "The demo status enum.") {
			t.Fatalf("status parameter description = %q, want API-facing enum type comment", parameter.Value.Description)
		}
		if !strings.Contains(parameter.Value.Description, "- `active`: the record is active") {
			t.Fatalf("status parameter description = %q, want enum value comments", parameter.Value.Description)
		}
		return
	}
	t.Fatal("status query parameter was not added")
}

// openapiNestedQueryItem is the element of a filterable collection, holding a
// slice of its own.
type openapiNestedQueryItem struct {
	Path    string   `json:"path"`
	Methods []string `json:"methods"`
}

// openapiNestedQueryModel filters on a field whose element type is a struct, so
// the query schema has to describe that struct's own members.
type openapiNestedQueryModel struct {
	// Items is a filterable collection of structs.
	Items []openapiNestedQueryItem `json:"items" query:"items"`

	modelregistry.Base
}

// TestAddQueryParametersDescribesSliceMembersOfNestedStructs asserts that a
// slice member of a nested struct is described as an array. OpenAPI 3.0 defines
// no "null" type, so emitting one makes the parameter schema invalid.
func TestAddQueryParametersDescribesSliceMembersOfNestedStructs(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParameters[*openapiNestedQueryModel, *openapiNestedQueryModel, *openapiNestedQueryModel](op)

	items := queryParametersByName(t, op)["items"]
	if items == nil || items.Schema == nil || items.Schema.Value == nil {
		t.Fatal("items query parameter schema is missing")
	}
	element := items.Schema.Value.Items
	if element == nil || element.Value == nil {
		t.Fatal("items element schema is missing")
	}
	methods := element.Value.Properties["methods"]
	if methods == nil || methods.Value == nil {
		t.Fatal("methods property schema is missing")
	}
	if methods.Value.Type == nil || !methods.Value.Type.Is(openapi3.TypeArray) {
		t.Fatalf("methods schema type = %v, want array", methods.Value.Type)
	}
}
