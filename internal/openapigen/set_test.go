package openapigen

import (
	"reflect"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/hydroan/gst/model"
)

type openapiTimeQueryModel struct {
	ExpiresAt *time.Time `json:"expires_at,omitempty" query:"expires_at"`

	model.Base
}

func TestSchemaFromTypeUsesDateTimeFormatForTime(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
	}{
		{
			name: "time value",
			typ:  reflect.TypeFor[time.Time](),
		},
		{
			name: "time pointer",
			typ:  reflect.TypeFor[*time.Time](),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaRef := schemaFromType(tt.typ)
			assertDateTimeSchema(t, schemaRef)
		})
	}
}

func TestSchemaFromTypeUsesDateTimeFormatForStructFields(t *testing.T) {
	schemaRef := schemaFromType(reflect.TypeFor[openapiTimeQueryModel]())
	if schemaRef == nil || schemaRef.Value == nil {
		t.Fatal("schemaFromType() returned nil schema")
	}

	expiresAt := schemaRef.Value.Properties["expires_at"]
	assertDateTimeSchema(t, expiresAt)
}

func TestAddQueryParametersUsesDateTimeFormatForTimeFields(t *testing.T) {
	op := &openapi3.Operation{}

	addQueryParameters[*openapiTimeQueryModel, *openapiTimeQueryModel, *openapiTimeQueryModel](op)

	for _, parameter := range op.Parameters {
		if parameter.Value == nil || parameter.Value.Name != "expires_at" {
			continue
		}

		assertDateTimeSchema(t, parameter.Value.Schema)
		return
	}

	t.Fatal("expires_at query parameter was not added")
}

func TestSchemaFromTypeKeepsRegularStructsAsObjects(t *testing.T) {
	type regularStruct struct {
		Name string `json:"name"`
	}

	schemaRef := schemaFromType(reflect.TypeFor[regularStruct]())
	if schemaRef == nil || schemaRef.Value == nil {
		t.Fatal("schemaFromType() returned nil schema")
	}
	if schemaRef.Value.Type == nil || !schemaRef.Value.Type.Is(openapi3.TypeObject) {
		t.Fatalf("schema type = %v, want object", schemaRef.Value.Type)
	}
}

type mapTitleModel struct {
	// GroupRoles binds groups to their roles.
	GroupRoles map[string][]string `json:"group_roles,omitempty"`
}

func TestAddSchemaFieldDocsSetsTitleAndDescription(t *testing.T) {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(mapTitleModel{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaFieldDocs[mapTitleModel](schemaRef)

	groupRoles := schemaRef.Value.Properties["group_roles"]
	if groupRoles == nil || groupRoles.Value == nil {
		t.Fatal("group_roles property missing")
	}
	if groupRoles.Value.Title != "GroupRoles binds groups to their roles." {
		t.Fatalf("group_roles title = %q, want field doc comment", groupRoles.Value.Title)
	}
	if groupRoles.Value.Description != "GroupRoles binds groups to their roles." {
		t.Fatalf("group_roles description = %q, want field doc comment", groupRoles.Value.Description)
	}
}

func TestAddSchemaFieldDocsDoesNotDuplicateIntoMapAdditionalProperties(t *testing.T) {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(mapTitleModel{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaFieldDocs[mapTitleModel](schemaRef)

	groupRoles := schemaRef.Value.Properties["group_roles"]
	if groupRoles == nil || groupRoles.Value == nil {
		t.Fatal("group_roles property missing")
	}

	// The map's additionalProperties value schema should stay undecorated: the
	// field-level docs above already describe it, and every other type in this
	// codebase (arrays, nested structs) only carries docs at the field level.
	additionalProperties := groupRoles.Value.AdditionalProperties.Schema
	if additionalProperties == nil || additionalProperties.Value == nil {
		t.Fatal("group_roles additionalProperties schema missing")
	}
	if additionalProperties.Value.Title != "" {
		t.Fatalf("additionalProperties title = %q, want empty", additionalProperties.Value.Title)
	}
	if additionalProperties.Value.Description != "" {
		t.Fatalf("additionalProperties description = %q, want empty", additionalProperties.Value.Description)
	}
}

type exampleItem struct {
	Name string `json:"name"`
}

type exampleNestedModel struct {
	Tags       map[string]int         `json:"tags"`
	GroupRoles map[string][]string    `json:"group_roles"`
	Metadata   map[string]exampleItem `json:"metadata"`
	Items      []exampleItem          `json:"items"`
}

func TestSetupExampleGeneratesRecursiveExamplesForArbitraryTypes(t *testing.T) {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(exampleNestedModel{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	setupExample(schemaRef)

	example, ok := schemaRef.Value.Example.(map[string]any)
	if !ok {
		t.Fatalf("example type = %T, want map[string]any", schemaRef.Value.Example)
	}

	tests := []struct {
		field string
		want  any
	}{
		{"tags", map[string]any{"string": 0}},
		{"group_roles", map[string]any{"string": []any{"string"}}},
		{"metadata", map[string]any{"string": map[string]any{"name": "string"}}},
		{"items", []any{map[string]any{"name": "string"}}},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got, ok := example[tt.field]
			if !ok {
				t.Fatalf("%s example missing", tt.field)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("%s example = %#v, want %#v", tt.field, got, tt.want)
			}
		})
	}
}

func TestSetupBatchExampleGeneratesRecursiveMapExample(t *testing.T) {
	type batchItemModel struct {
		GroupRoles map[string][]string `json:"group_roles"`
	}
	type batchRequest struct {
		Items []batchItemModel `json:"items"`
	}

	schemaRef, err := openapi3gen.NewSchemaRefForValue(batchRequest{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	setupBatchExample(schemaRef)

	itemsProp := schemaRef.Value.Properties["items"]
	if itemsProp == nil || itemsProp.Value == nil || itemsProp.Value.Items == nil || itemsProp.Value.Items.Value == nil {
		t.Fatal("items schema missing")
	}

	example, ok := itemsProp.Value.Items.Value.Example.(map[string]any)
	if !ok {
		t.Fatalf("item example type = %T, want map[string]any", itemsProp.Value.Items.Value.Example)
	}

	want := map[string]any{"string": []any{"string"}}
	got, ok := example["group_roles"]
	if !ok {
		t.Fatal("group_roles example missing")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("group_roles example = %#v, want %#v", got, want)
	}
}

func assertDateTimeSchema(t *testing.T, schemaRef *openapi3.SchemaRef) {
	t.Helper()

	if schemaRef == nil || schemaRef.Value == nil {
		t.Fatal("schema is nil")
	}
	if schemaRef.Value.Type == nil || !schemaRef.Value.Type.Is(openapi3.TypeString) {
		t.Fatalf("schema type = %v, want string", schemaRef.Value.Type)
	}
	if schemaRef.Value.Format != "date-time" {
		t.Fatalf("schema format = %q, want date-time", schemaRef.Value.Format)
	}
}
