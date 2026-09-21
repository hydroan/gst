package openapigen

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/internal/modelregistry"
)

// registerFixtureDoc registers the doc comments the generator is expected to
// read for a fixture declared in this package. The generator reads the apidoc
// registry and nothing else, so a fixture documents itself here instead of
// through a Go doc comment, exactly like a generated model/apidoc.gen.go
// documents a project's models.
func registerFixtureDoc(typeName, comment string, fields map[string]string) {
	apidoc.Register(reflect.TypeFor[openapiTimeQueryModel]().PkgPath(), typeName, apidoc.StructDoc{
		Comment: comment,
		Fields:  fields,
	})
}

// Fixtures shared by more than one test file. Fixtures used by a single file
// live next to the test that uses them.

type openapiTimeQueryModel struct {
	ExpiresAt *time.Time `json:"expires_at,omitempty" query:"expires_at"`

	modelregistry.Base
}

func init() {
	registerFixtureDoc("openapiTimeQueryModel", "", map[string]string{
		"ExpiresAt": "ExpiresAt is the expiration time.",
	})
}

type openapiEmbeddedQueryModel struct {
	// Page is a business filter field; the bare name no longer collides with
	// the framework pagination parameter, which lives in the "_" namespace.
	Page string `json:"-" query:"page"`

	modelregistry.Query
	modelregistry.Base
}

// enumFieldStatus is a demo enum type for schema decoration tests.
type enumFieldStatus string

func registerEnumFieldStatus() {
	apidoc.RegisterEnum(reflect.TypeFor[enumFieldStatus]().PkgPath(), "enumFieldStatus", apidoc.EnumDoc{
		Comment: "enumFieldStatus is the demo status enum.",
		Values: []apidoc.EnumValue{
			{Value: "active", Comment: "the record is active"},
			{Value: "disabled", Comment: "the record is disabled"},
		},
	})
}

// Assertions and document lookups shared by more than one test file.

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

func operationForPath(t *testing.T, path string) *openapi3.Operation {
	t.Helper()

	item := doc.Paths.Value(path)
	if item == nil || item.Post == nil {
		t.Fatalf("POST %s is missing from the document", path)
	}
	return item.Post
}

func responseRefForPath(t *testing.T, path string, status int) *openapi3.ResponseRef {
	t.Helper()

	item := doc.Paths.Value(path)
	if item == nil {
		t.Fatalf("%s is missing from the document", path)
	}
	op := item.Post
	if op == nil {
		op = item.Get
	}
	if op == nil {
		t.Fatalf("%s has no POST or GET operation", path)
	}
	return op.Responses.Status(status)
}

func registeredRequestBodySchema(t *testing.T, requestBodyRef *openapi3.RequestBodyRef) *openapi3.Schema {
	t.Helper()

	if requestBodyRef == nil {
		t.Fatal("operation request body is missing")
	}
	if requestBodyRef.Ref != "" {
		reqKey := strings.TrimPrefix(requestBodyRef.Ref, "#/components/requestBodies/")
		docMutex.RLock()
		requestBodyRef = doc.Components.RequestBodies[reqKey]
		docMutex.RUnlock()
	}
	if requestBodyRef == nil || requestBodyRef.Value == nil {
		t.Fatal("registered request body component is missing")
	}
	mediaType := requestBodyRef.Value.Content["application/json"]
	if mediaType == nil || mediaType.Schema == nil || mediaType.Schema.Value == nil {
		t.Fatal("registered request body component JSON schema is missing")
	}
	return mediaType.Schema.Value
}

func registeredResponseSchema(t *testing.T, responseRef *openapi3.ResponseRef) *openapi3.Schema {
	t.Helper()

	if responseRef == nil {
		t.Fatal("operation response is missing")
	}
	if responseRef.Ref != "" {
		rspKey := strings.TrimPrefix(responseRef.Ref, "#/components/responses/")
		docMutex.RLock()
		responseRef = doc.Components.Responses[rspKey]
		docMutex.RUnlock()
	}
	if responseRef == nil || responseRef.Value == nil {
		t.Fatal("registered response component is missing")
	}
	mediaType := responseRef.Value.Content["application/json"]
	if mediaType == nil || mediaType.Schema == nil || mediaType.Schema.Value == nil {
		t.Fatal("registered response component JSON schema is missing")
	}
	return mediaType.Schema.Value
}

func dataSchema(t *testing.T, envelope *openapi3.Schema) *openapi3.Schema {
	t.Helper()

	data := envelope.Properties["data"]
	if data == nil || data.Value == nil {
		t.Fatalf("response envelope = %v, want a data property", propertyNames(envelope))
	}
	return data.Value
}

func propertyNames(schema *openapi3.Schema) []string {
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func queryParametersByName(t *testing.T, op *openapi3.Operation) map[string]*openapi3.Parameter {
	t.Helper()

	parameters := make(map[string]*openapi3.Parameter, len(op.Parameters))
	for _, parameterRef := range op.Parameters {
		if parameterRef == nil || parameterRef.Value == nil || parameterRef.Value.In != "query" {
			continue
		}
		name := parameterRef.Value.Name
		if parameters[name] != nil {
			t.Fatalf("query parameter %q was added more than once", name)
		}
		parameters[name] = parameterRef.Value
	}
	return parameters
}

func parameterNames(parameters map[string]*openapi3.Parameter) []string {
	names := make([]string, 0, len(parameters))
	for name := range parameters {
		names = append(names, name)
	}
	return names
}
