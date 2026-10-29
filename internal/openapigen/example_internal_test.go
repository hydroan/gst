package openapigen

import (
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3gen"
)

type exampleItem struct {
	Name string `json:"name"`
}

type exampleNestedModel struct {
	Tags     map[string]int         `json:"tags"`
	ItemTags map[string][]string    `json:"item_tags"`
	Metadata map[string]exampleItem `json:"metadata"`
	Items    []exampleItem          `json:"items"`
}

func TestApplyExampleGeneratesRecursiveExamplesForArbitraryTypes(t *testing.T) {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(exampleNestedModel{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	applyExample(schemaRef)

	example, ok := schemaRef.Value.Example.(map[string]any)
	if !ok {
		t.Fatalf("example type = %T, want map[string]any", schemaRef.Value.Example)
	}

	tests := []struct {
		field string
		want  any
	}{
		{"tags", map[string]any{"string": 0}},
		{"item_tags", map[string]any{"string": []any{"string"}}},
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

// TestApplyExampleIllustratesEveryProperty asserts that the example shows every
// property the schema documents, whatever its name, at the top level and
// nested: hiding the fields a write takes no value for is the request body's
// business (see hideUnwrittenFields), done before the example is built.
func TestApplyExampleIllustratesEveryProperty(t *testing.T) {
	type nestedItem struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type parentRequest struct {
		ID     string       `json:"id"`
		Label  string       `json:"label"`
		Single *nestedItem  `json:"single"`
		Items  []nestedItem `json:"items"`
	}

	schemaRef, err := openapi3gen.NewSchemaRefForValue(parentRequest{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	applyExample(schemaRef)

	want := map[string]any{
		"id":     "string",
		"label":  "string",
		"single": map[string]any{"id": "string", "name": "string"},
		"items":  []any{map[string]any{"id": "string", "name": "string"}},
	}
	if !reflect.DeepEqual(schemaRef.Value.Example, want) {
		t.Fatalf("example = %#v, want %#v", schemaRef.Value.Example, want)
	}
}

// TestApplyExampleIllustratesTheRecordOfABatch asserts that the example of a
// batch body shows its items array holding one record, every field of the
// record illustrated, a map of slices included.
func TestApplyExampleIllustratesTheRecordOfABatch(t *testing.T) {
	type batchItemModel struct {
		ItemTags map[string][]string `json:"item_tags"`
	}
	type batchRequest struct {
		Items []batchItemModel `json:"items"`
	}

	schemaRef, err := openapi3gen.NewSchemaRefForValue(batchRequest{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	applyExample(schemaRef)

	want := map[string]any{
		"items": []any{map[string]any{"item_tags": map[string]any{"string": []any{"string"}}}},
	}
	if !reflect.DeepEqual(schemaRef.Value.Example, want) {
		t.Fatalf("example = %#v, want %#v", schemaRef.Value.Example, want)
	}
}
