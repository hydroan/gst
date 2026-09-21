package openapigen

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/hydroan/gst/apidoc"
	"gorm.io/datatypes"
)

// The doc comments the schema tests expect on the fixtures declared in this
// file. They are registered rather than written as Go doc comments because the
// registry is the only thing the generator reads.
func init() {
	registerFixtureDoc("mapDocModel", "", map[string]string{
		"GroupRoles": "GroupRoles binds groups to their roles.",
	})
	registerFixtureDoc("jsonDocPayload", "", map[string]string{
		"Code": "Code is the payload code.",
	})
	registerFixtureDoc("jsonDocModel", "", map[string]string{
		"Config": "Config is the configuration payload.",
	})
	registerFixtureDoc("enumFieldModel", "", map[string]string{
		"Status": "Status is the record status.",
	})
	registerFixtureDoc("nestedPayloadOption", "", map[string]string{
		"Code": "Code is the option code.",
		"Name": "Name is the option name.",
	})
	registerFixtureDoc("nestedPayloadReq", "", map[string]string{
		"MaxAmount": "MaxAmount is the request level limit.",
		"Options":   "Options is the full nested option collection.",
	})
	registerFixtureDoc("embeddedSchemeRow", "", map[string]string{
		"Status": "Status is the record status.",
		"Label":  "Label is the row label.",
	})
	registerFixtureDoc("embeddedSchemeView", "", map[string]string{
		"Options": "Options is the nested option collection of the row.",
	})
	registerFixtureDoc("cyclicCategory", "", map[string]string{
		"Title": "Title is the category title.",
	})
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

type mapDocModel struct {
	GroupRoles map[string][]string `json:"group_roles,omitempty"`
}

func TestAddSchemaDocsForTypeSetsDescriptionWithoutTitle(t *testing.T) {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(mapDocModel{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaDocsForType(reflect.TypeFor[mapDocModel](), schemaRef, nil)

	groupRoles := schemaRef.Value.Properties["group_roles"]
	if groupRoles == nil || groupRoles.Value == nil {
		t.Fatal("group_roles property missing")
	}
	if groupRoles.Value.Title != "" {
		t.Fatalf("group_roles title = %q, want empty", groupRoles.Value.Title)
	}
	if groupRoles.Value.Description != "Binds groups to their roles." {
		t.Fatalf("group_roles description = %q, want API-facing field comment", groupRoles.Value.Description)
	}
}

func TestAddSchemaDocsForTypePreservesExistingTitle(t *testing.T) {
	property := openapi3.NewStringSchema()
	property.Title = "Explicit schema title"
	object := openapi3.NewObjectSchema()
	object.WithProperty("group_roles", property)
	schemaRef := &openapi3.SchemaRef{Value: object}

	addSchemaDocsForType(reflect.TypeFor[mapDocModel](), schemaRef, nil)

	groupRoles := schemaRef.Value.Properties["group_roles"]
	if groupRoles == nil || groupRoles.Value == nil {
		t.Fatal("group_roles property missing")
	}
	if groupRoles.Value.Title != "Explicit schema title" {
		t.Fatalf("group_roles title = %q, want explicit schema title", groupRoles.Value.Title)
	}
	if groupRoles.Value.Description != "Binds groups to their roles." {
		t.Fatalf("group_roles description = %q, want API-facing field comment", groupRoles.Value.Description)
	}
}

func TestAddSchemaDocsForTypeDoesNotDuplicateIntoMapAdditionalProperties(t *testing.T) {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(mapDocModel{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaDocsForType(reflect.TypeFor[mapDocModel](), schemaRef, nil)

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

type jsonDocPayload struct {
	Code string `json:"code"`
}

type jsonDocModel struct {
	Config datatypes.JSONType[jsonDocPayload] `json:"config"`
}

func TestAddSchemaDocsForTypeDecoratesJSONTypeWithoutTitle(t *testing.T) {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(jsonDocModel{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaDocsForType(reflect.TypeFor[jsonDocModel](), schemaRef, nil)

	config := schemaRef.Value.Properties["config"]
	if config == nil || config.Value == nil {
		t.Fatal("config property missing")
	}
	if config.Value.Title != "" {
		t.Fatalf("config title = %q, want empty", config.Value.Title)
	}
	if config.Value.Description != "The configuration payload." {
		t.Fatalf("config description = %q, want API-facing field comment", config.Value.Description)
	}

	code := config.Value.Properties["code"]
	if code == nil || code.Value == nil {
		t.Fatal("config code property missing")
	}
	if code.Value.Title != "" {
		t.Fatalf("config code title = %q, want empty", code.Value.Title)
	}
	if code.Value.Description != "The payload code." {
		t.Fatalf("config code description = %q, want API-facing nested field comment", code.Value.Description)
	}
}

type enumFieldModel struct {
	Status enumFieldStatus `json:"status"`

	Codes []enumFieldStatus `json:"codes"`
}

func TestAddSchemaDocsForTypeSetsEnumValues(t *testing.T) {
	registerEnumFieldStatus()

	schemaRef, err := openapi3gen.NewSchemaRefForValue(enumFieldModel{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaDocsForType(reflect.TypeFor[enumFieldModel](), schemaRef, nil)

	status := schemaRef.Value.Properties["status"]
	if status == nil || status.Value == nil {
		t.Fatal("status property missing")
	}
	if len(status.Value.Enum) != 2 || status.Value.Enum[0] != "active" || status.Value.Enum[1] != "disabled" {
		t.Fatalf("status enum = %#v, want [active disabled]", status.Value.Enum)
	}
	if !strings.Contains(status.Value.Description, "The record status.") {
		t.Fatalf("status description = %q, want it to keep the field comment", status.Value.Description)
	}
	if !strings.Contains(status.Value.Description, "- `active`: the record is active") {
		t.Fatalf("status description = %q, want it to list enum value comments", status.Value.Description)
	}
}

func TestAddSchemaDocsForTypeSetsEnumOnSliceItemsWithoutFieldComment(t *testing.T) {
	registerEnumFieldStatus()

	schemaRef, err := openapi3gen.NewSchemaRefForValue(enumFieldModel{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaDocsForType(reflect.TypeFor[enumFieldModel](), schemaRef, nil)

	codes := schemaRef.Value.Properties["codes"]
	if codes == nil || codes.Value == nil {
		t.Fatal("codes property missing")
	}
	if codes.Value.Items == nil || codes.Value.Items.Value == nil {
		t.Fatal("codes items schema missing")
	}
	if len(codes.Value.Items.Value.Enum) != 2 {
		t.Fatalf("codes items enum = %#v, want the two enum values", codes.Value.Items.Value.Enum)
	}
	// The field has no comment, so the enum type comment becomes the base text.
	if !strings.Contains(codes.Value.Description, "The demo status enum.") {
		t.Fatalf("codes description = %q, want the enum type comment as base text", codes.Value.Description)
	}
}

// nestedPayloadOption is one nested payload item for schema decoration tests.
type nestedPayloadOption struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// nestedPayloadReq is a request body carrying a nested item collection.
type nestedPayloadReq struct {
	MaxAmount int64                  `json:"max_amount"`
	Options   []*nestedPayloadOption `json:"options"`
}

func TestAddSchemaDocsForTypeDecoratesNestedStructFields(t *testing.T) {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(nestedPayloadReq{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaDocsForType(reflect.TypeFor[nestedPayloadReq](), schemaRef, nil)

	options := schemaRef.Value.Properties["options"]
	if options == nil || options.Value == nil {
		t.Fatal("options property missing")
	}
	if options.Value.Title != "" {
		t.Fatalf("options title = %q, want empty", options.Value.Title)
	}
	if options.Value.Description != "The full nested option collection." {
		t.Fatalf("options description = %q, want API-facing field comment", options.Value.Description)
	}
	if options.Value.Items == nil || options.Value.Items.Value == nil {
		t.Fatal("options items schema missing")
	}

	code := options.Value.Items.Value.Properties["code"]
	if code == nil || code.Value == nil {
		t.Fatal("options items code property missing")
	}
	if code.Value.Title != "" {
		t.Fatalf("nested code title = %q, want empty", code.Value.Title)
	}
	if code.Value.Description != "The option code." {
		t.Fatalf("nested code description = %q, want API-facing nested struct field comment", code.Value.Description)
	}
}

func TestAddSchemaDocsForTypeDecoratesAnonymousStructBySignature(t *testing.T) {
	apidoc.Register("openapigen/anon", "anonSchemaPayloadDoc", apidoc.StructDoc{
		Fields: map[string]string{
			"Headline": "The headline.",
			"Slug":     "The slug.",
		},
	})

	type anonSchemaPayload = struct {
		Headline string `json:"headline"`
		Slug     string `json:"slug"`
	}

	schemaRef, err := openapi3gen.NewSchemaRefForValue(anonSchemaPayload{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaDocsForType(reflect.TypeFor[anonSchemaPayload](), schemaRef, nil)

	headline := schemaRef.Value.Properties["headline"]
	if headline == nil || headline.Value == nil {
		t.Fatal("headline property missing")
	}
	if headline.Value.Description != "The headline." {
		t.Fatalf("headline description = %q, want anonymous struct field comment", headline.Value.Description)
	}

	slug := schemaRef.Value.Properties["slug"]
	if slug == nil || slug.Value == nil {
		t.Fatal("slug property missing")
	}
	if slug.Value.Description != "The slug." {
		t.Fatalf("slug description = %q, want anonymous struct field comment", slug.Value.Description)
	}
}

// embeddedSchemeRow is the persisted row promoted into view structs.
type embeddedSchemeRow struct {
	Status enumFieldStatus `json:"status"`
	Label  string          `json:"label"`
}

// embeddedSchemeView is a response view embedding the persisted row.
type embeddedSchemeView struct {
	*embeddedSchemeRow
	Options []*nestedPayloadOption `json:"options"`
}

func TestAddSchemaDocsForTypeDecoratesEmbeddedStructFields(t *testing.T) {
	registerEnumFieldStatus()

	schemaRef, err := openapi3gen.NewSchemaRefForValue(embeddedSchemeView{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaDocsForType(reflect.TypeFor[embeddedSchemeView](), schemaRef, nil)

	label := schemaRef.Value.Properties["label"]
	if label == nil || label.Value == nil {
		t.Fatal("label property missing")
	}
	if label.Value.Title != "" {
		t.Fatalf("promoted label title = %q, want empty", label.Value.Title)
	}
	if label.Value.Description != "The row label." {
		t.Fatalf("promoted label description = %q, want API-facing embedded struct field comment", label.Value.Description)
	}

	status := schemaRef.Value.Properties["status"]
	if status == nil || status.Value == nil {
		t.Fatal("status property missing")
	}
	if len(status.Value.Enum) != 2 {
		t.Fatalf("promoted status enum = %#v, want the two enum values", status.Value.Enum)
	}
	if !strings.Contains(status.Value.Description, "The record status.") {
		t.Fatalf("promoted status description = %q, want embedded struct field doc comment", status.Value.Description)
	}
}

// cyclicCategory is a self referential type for the cycle guard test.
type cyclicCategory struct {
	Title string `json:"title"`

	Children []*cyclicCategory `json:"children"`
}

func TestAddSchemaDocsForTypeCutsCyclicStructTypes(t *testing.T) {
	// Build a self referencing schema by hand: the children items schema points
	// back at the root, mirroring the cyclic Go type. Without the cycle guard
	// the walk would recurse forever.
	object := openapi3.NewObjectSchema()
	rootRef := &openapi3.SchemaRef{Value: object}
	children := openapi3.NewArraySchema()
	children.Items = rootRef
	object.WithProperty("title", openapi3.NewStringSchema())
	object.WithPropertyRef("children", &openapi3.SchemaRef{Value: children})

	addSchemaDocsForType(reflect.TypeFor[cyclicCategory](), rootRef, nil)

	title := rootRef.Value.Properties["title"]
	if title == nil || title.Value == nil {
		t.Fatal("title property missing")
	}
	if title.Value.Title != "" {
		t.Fatalf("title schema title = %q, want empty", title.Value.Title)
	}
	if title.Value.Description != "The category title." {
		t.Fatalf("title description = %q, want API-facing field comment", title.Value.Description)
	}
}

// customListRsp is a custom list response whose shape mimics the framework
// list wrapper (items plus total), so wrapper shape sniffing would misfire.
type customListRsp struct {
	Items []*embeddedSchemeView `json:"items"`
	Total int64                 `json:"total"`
}

func TestNewSchemaRefWithDocsDecoratesCustomListResponseWrapper(t *testing.T) {
	registerEnumFieldStatus()

	schemaRef := newSchemaRefWithDocs(apiResponse[*customListRsp]{})
	if schemaRef == nil || schemaRef.Value == nil {
		t.Fatal("newSchemaRefWithDocs() returned nil schema")
	}

	data := schemaRef.Value.Properties["data"]
	if data == nil || data.Value == nil {
		t.Fatal("data property missing")
	}
	items := data.Value.Properties["items"]
	if items == nil || items.Value == nil || items.Value.Items == nil || items.Value.Items.Value == nil {
		t.Fatal("data items schema missing")
	}

	element := items.Value.Items.Value
	status := element.Properties["status"]
	if status == nil || status.Value == nil {
		t.Fatal("items element status property missing")
	}
	if len(status.Value.Enum) != 2 {
		t.Fatalf("items element status enum = %#v, want the two enum values", status.Value.Enum)
	}
	if !strings.Contains(status.Value.Description, "The record status.") {
		t.Fatalf("items element status description = %q, want embedded struct field doc comment", status.Value.Description)
	}

	options := element.Properties["options"]
	if options == nil || options.Value == nil || options.Value.Items == nil || options.Value.Items.Value == nil {
		t.Fatal("items element options schema missing")
	}
	name := options.Value.Items.Value.Properties["name"]
	if name == nil || name.Value == nil {
		t.Fatal("deeply nested option name property missing its doc comment")
	}
	if name.Value.Title != "" {
		t.Fatalf("deeply nested option name title = %q, want empty", name.Value.Title)
	}
	if name.Value.Description != "The option name." {
		t.Fatalf("deeply nested option name description = %q, want API-facing field comment", name.Value.Description)
	}
}
