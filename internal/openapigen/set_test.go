package openapigen

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types/consts"
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

func TestAddSchemaDocsForTypeSetsTitleAndDescription(t *testing.T) {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(mapTitleModel{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaDocsForType(reflect.TypeFor[mapTitleModel](), schemaRef, nil)

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

func TestAddSchemaDocsForTypeDoesNotDuplicateIntoMapAdditionalProperties(t *testing.T) {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(mapTitleModel{}, nil)
	if err != nil {
		t.Fatalf("NewSchemaRefForValue() error = %v", err)
	}

	addSchemaDocsForType(reflect.TypeFor[mapTitleModel](), schemaRef, nil)

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

// enumFieldStatus is a demo enum type for schema decoration tests.
type enumFieldStatus string

type enumFieldModel struct {
	// Status is the record status.
	Status enumFieldStatus `json:"status"`

	Codes []enumFieldStatus `json:"codes"`
}

func registerEnumFieldStatus() {
	apidoc.RegisterEnum(reflect.TypeFor[enumFieldStatus]().PkgPath(), "enumFieldStatus", apidoc.EnumDoc{
		Comment: "enumFieldStatus is the demo status enum.",
		Values: []apidoc.EnumValue{
			{Value: "active", Comment: "the record is active"},
			{Value: "disabled", Comment: "the record is disabled"},
		},
	})
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
	if !strings.Contains(status.Value.Description, "Status is the record status.") {
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
	if !strings.Contains(codes.Value.Description, "enumFieldStatus is the demo status enum.") {
		t.Fatalf("codes description = %q, want the enum type comment as base text", codes.Value.Description)
	}
}

// nestedBetOption is one nested payload item for schema decoration tests.
type nestedBetOption struct {
	// Code is the option code.
	Code string `json:"code"`
	// Name is the option name.
	Name string `json:"name"`
}

// nestedPayloadReq is a request body carrying a nested item collection.
type nestedPayloadReq struct {
	// MaxAmount is the request level limit.
	MaxAmount int64 `json:"max_amount"`
	// Options is the full nested option collection.
	Options []*nestedBetOption `json:"options"`
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
	if options.Value.Title != "Options is the full nested option collection." {
		t.Fatalf("options title = %q, want field doc comment", options.Value.Title)
	}
	if options.Value.Items == nil || options.Value.Items.Value == nil {
		t.Fatal("options items schema missing")
	}

	code := options.Value.Items.Value.Properties["code"]
	if code == nil || code.Value == nil {
		t.Fatal("options items code property missing")
	}
	if code.Value.Title != "Code is the option code." {
		t.Fatalf("nested code title = %q, want nested struct field doc comment", code.Value.Title)
	}
	if code.Value.Description != "Code is the option code." {
		t.Fatalf("nested code description = %q, want nested struct field doc comment", code.Value.Description)
	}
}

// embeddedSchemeRow is the persisted row promoted into view structs.
type embeddedSchemeRow struct {
	// Status is the record status.
	Status enumFieldStatus `json:"status"`
	// Label is the row label.
	Label string `json:"label"`
}

// embeddedSchemeView is a response view embedding the persisted row.
type embeddedSchemeView struct {
	*embeddedSchemeRow
	// Options is the nested option collection of the row.
	Options []*nestedBetOption `json:"options"`
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
	if label.Value.Title != "Label is the row label." {
		t.Fatalf("promoted label title = %q, want embedded struct field doc comment", label.Value.Title)
	}

	status := schemaRef.Value.Properties["status"]
	if status == nil || status.Value == nil {
		t.Fatal("status property missing")
	}
	if len(status.Value.Enum) != 2 {
		t.Fatalf("promoted status enum = %#v, want the two enum values", status.Value.Enum)
	}
	if !strings.Contains(status.Value.Description, "Status is the record status.") {
		t.Fatalf("promoted status description = %q, want embedded struct field doc comment", status.Value.Description)
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

	schemaRef := newSchemaRefWithDocs(*new(apiResponse[*customListRsp]))
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
	if !strings.Contains(status.Value.Description, "Status is the record status.") {
		t.Fatalf("items element status description = %q, want embedded struct field doc comment", status.Value.Description)
	}

	options := element.Properties["options"]
	if options == nil || options.Value == nil || options.Value.Items == nil || options.Value.Items.Value == nil {
		t.Fatal("items element options schema missing")
	}
	name := options.Value.Items.Value.Properties["name"]
	if name == nil || name.Value == nil || name.Value.Title != "Name is the option name." {
		t.Fatal("deeply nested option name property missing its doc comment")
	}
}

// cyclicCategory is a self referential type for the cycle guard test.
type cyclicCategory struct {
	// Title is the category title.
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
	if title.Value.Title != "Title is the category title." {
		t.Fatalf("title = %q, want field doc comment", title.Value.Title)
	}
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
		if !strings.Contains(parameter.Value.Description, "- `active`: the record is active") {
			t.Fatalf("status parameter description = %q, want enum value comments", parameter.Value.Description)
		}
		return
	}
	t.Fatal("status query parameter was not added")
}

type enumQueryModel struct {
	Status enumFieldStatus `json:"status" query:"status"`

	model.Base
}

func TestSchemaComponentName(t *testing.T) {
	tests := []struct {
		name    string
		pkgPath string
		typName string
		want    string
	}{
		{"model subpackage", "dice/model/play", "Customization", "play.Customization"},
		{"nested model subpackage", "github.com/hydroan/gst/internal/model/iam/user", "User", "iam.user.User"},
		{"model root", "dice/model", "User", "User"},
		{"non-model package", "github.com/hydroan/gst/module/mfa", "TOTPBind", "module.mfa.TOTPBind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schemaComponentNameFromPath(tt.pkgPath, tt.typName)
			if got != tt.want {
				t.Fatalf("schemaComponentName(%s.%s) = %q, want %q", tt.pkgPath, tt.typName, got, tt.want)
			}
		})
	}
}

func TestOperationIDDerivesFromPath(t *testing.T) {
	tests := []struct {
		path string
		op   consts.HTTPVerb
		want string
	}{
		{"/api/play/customizations/{id}", consts.Patch, "play_customizations_patch"},
		{"/api/groups", consts.List, "groups_list"},
		{"/api/hello-world", consts.Create, "hello_world_create"},
		{"/api/groups/{group}/players", consts.List, "groups_players_list"},
	}
	for _, tt := range tests {
		if got := operationID(tt.path, tt.op); got != tt.want {
			t.Fatalf("operationID(%s, %s) = %q, want %q", tt.path, tt.op, got, tt.want)
		}
	}
}

// summaryFirstLineModel is the human readable summary line.
// The second comment line must not leak into the summary.
type summaryFirstLineModel struct {
	Name string `json:"name"`
}

func TestSummaryPrefersStructCommentFirstLine(t *testing.T) {
	typ := reflect.TypeFor[*summaryFirstLineModel]()
	got := summary("/api/play/customizations", consts.Patch, typ)
	if got != "summaryFirstLineModel is the human readable summary line." {
		t.Fatalf("summary() = %q, want the first comment line", got)
	}
}

func TestSummaryFallsBackToPathToken(t *testing.T) {
	typ := reflect.TypeOf(&struct{ Name string }{})
	got := summary("/api/play/customizations/{id}", consts.Patch, typ)
	if got != "customizations_patch" {
		t.Fatalf("summary() = %q, want the mechanical fallback", got)
	}
}

func TestTagsSkipPathParameters(t *testing.T) {
	got := tags("/api/{tenant}/groups", consts.List, reflect.TypeFor[*summaryFirstLineModel]())
	if len(got) != 1 || got[0] != "groups" {
		t.Fatalf("tags() = %v, want [groups]", got)
	}
}

func TestUniqueComponentNameResolvesCrossPackageCollision(t *testing.T) {
	first := uniqueComponentName(reflect.TypeFor[openapiTimeQueryModel]())
	if first != "internal.openapigen.openapiTimeQueryModel" {
		t.Fatalf("first = %q, want the last-two-segment qualified name", first)
	}

	// A different package claiming the same base name must get a fully
	// qualified fallback instead of silently sharing the component.
	componentNameMu.Lock()
	componentNameOwners["internal.openapigen.openapiTimeQueryModel"] = "example.com/other/pkg"
	componentNameMu.Unlock()

	second := uniqueComponentName(reflect.TypeFor[openapiTimeQueryModel]())
	if second != "github.com.hydroan.gst.internal.openapigen.openapiTimeQueryModel" {
		t.Fatalf("second = %q, want the fully qualified fallback", second)
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
