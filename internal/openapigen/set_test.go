package openapigen

import (
	"testing"

	"gorm.io/datatypes"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
)

type jsonTypeConfig struct {
	Foo string `json:"foo"`
}

type jsonTypeModel struct {
	// config doc
	Config datatypes.JSONType[jsonTypeConfig] `json:"config"`
}

func TestConvertDatatypesJSONTypeSchema(t *testing.T) {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(jsonTypeModel{}, nil)
	if err != nil {
		t.Fatalf("generate schema: %v", err)
	}

	addSchemaTitle[jsonTypeModel](schemaRef)

	configRef, ok := schemaRef.Value.Properties["config"]
	if !ok {
		t.Fatalf("config property missing")
	}
	if configRef.Value == nil {
		t.Fatalf("config schema missing value")
	}
	if configRef.Value.Title != "config doc" {
		t.Fatalf("config title want %q got %q", "config doc", configRef.Value.Title)
	}
	fooRef, ok := configRef.Value.Properties["foo"]
	if !ok {
		t.Fatalf("foo property missing in underlying schema")
	}
	if fooRef.Value == nil || fooRef.Value.Type == nil || !fooRef.Value.Type.Is(openapi3.TypeString) {
		t.Fatalf("foo property should be string type")
	}
}
