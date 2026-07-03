package openapigen

import (
	"reflect"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
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
