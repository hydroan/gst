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
	"gorm.io/datatypes"
)

type openapiTimeQueryModel struct {
	// ExpiresAt is the expiration time.
	ExpiresAt *time.Time `json:"expires_at,omitempty" query:"expires_at"`

	model.Base
}

type openapiEmbeddedQueryModel struct {
	// Page overrides the promoted pagination field.
	Page string `json:"-" query:"page"`

	model.Query
	model.Base
}

type openapiPaginationQueryModel struct {
	model.Pagination
	model.Base
}

type openapiCursorQueryModel struct {
	model.Cursor
	model.Base
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
	model.Base
}

type openapiSliceQueryModel struct {
	Values []string `json:"-" query:"values"`

	model.Base
}

type openapiDefaultCreateModel struct {
	Name string `json:"name"`

	model.Base
}

type openapiCustomCreateModel struct {
	Name string `json:"name"`

	model.Base
}

type openapiCustomCreateRequest struct {
	Name string `json:"name"`
}

type openapiCustomCreateResponse struct {
	Result string `json:"result"`
}

type openapiCustomBatchModel struct {
	Name string `json:"name"`

	model.Base
}

type openapiCustomBatchRequest struct {
	Name string `json:"name"`
}

type openapiCustomBatchResponse struct {
	Result string `json:"result"`
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
		if parameter.Value.Description != "The expiration time." {
			t.Fatalf("expires_at description = %q, want API-facing field comment", parameter.Value.Description)
		}
		return
	}

	t.Fatal("expires_at query parameter was not added")
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
				"page", "size",
				"_cursor_value", "_cursor_fields", "_cursor_next",
				"_expand", "_depth", "_fuzzy", "_sortby", "_nocache",
				"_column_name", "_start_time", "_end_time", "_or", "_index",
				"_select", "_nototal",
				"id", "created_by", "updated_by",
			},
		},
		{
			name: "pagination",
			add: func(op *openapi3.Operation) {
				addQueryParameters[*openapiPaginationQueryModel, *openapiPaginationQueryModel, *openapiPaginationQueryModel](op)
			},
			parameters: []string{"page", "size", "id", "created_by", "updated_by"},
		},
		{
			name: "cursor",
			add: func(op *openapi3.Operation) {
				addQueryParameters[*openapiCursorQueryModel, *openapiCursorQueryModel, *openapiCursorQueryModel](op)
			},
			parameters: []string{"_cursor_value", "_cursor_fields", "_cursor_next", "id", "created_by", "updated_by"},
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
		t.Fatalf("page schema = %#v, want the outer string field to override the embedded pagination field", page.Schema)
	}

	op = &openapi3.Operation{}
	addQueryParameters[*openapiShallowQueryModel, *openapiShallowQueryModel, *openapiShallowQueryModel](op)
	shared := queryParametersByName(t, op)["shared"]
	if shared.Schema == nil || shared.Schema.Value == nil || shared.Schema.Value.Type == nil || !shared.Schema.Value.Type.Is(openapi3.TypeString) {
		t.Fatalf("shared schema = %#v, want the shallower embedded string field to override the earlier deeper field", shared.Schema)
	}
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

func TestSetCreateDocumentsRuntimeSuccessStatus(t *testing.T) {
	tests := []struct {
		name       string
		set        func(*openapi3.PathItem)
		wantStatus string
	}{
		{
			name: "default create",
			set: func(pathItem *openapi3.PathItem) {
				setCreate[*openapiDefaultCreateModel, *openapiDefaultCreateModel, *openapiDefaultCreateModel]("/api/default-create", pathItem)
			},
			wantStatus: "201",
		},
		{
			name: "custom create",
			set: func(pathItem *openapi3.PathItem) {
				setCreate[*openapiCustomCreateModel, openapiCustomCreateRequest, openapiCustomCreateResponse]("/api/custom-create", pathItem)
			},
			wantStatus: "200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathItem := &openapi3.PathItem{}
			tt.set(pathItem)

			if pathItem.Post == nil || pathItem.Post.Responses == nil || pathItem.Post.Responses.Value(tt.wantStatus) == nil {
				t.Fatalf("documented responses = %v, want status %s", pathItem.Post.Responses, tt.wantStatus)
			}
			wrongStatus := "201"
			if tt.wantStatus == "201" {
				wrongStatus = "200"
			}
			if pathItem.Post.Responses.Value(wrongStatus) != nil {
				t.Fatalf("documented responses unexpectedly include status %s", wrongStatus)
			}
		})
	}
}

func TestSetCustomBatchDocumentsResponseEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		set        func(*openapi3.PathItem)
		operation  func(*openapi3.PathItem) *openapi3.Operation
		wantStatus string
	}{
		{
			name: "create many",
			set: func(pathItem *openapi3.PathItem) {
				setCreateMany[*openapiCustomBatchModel, openapiCustomBatchRequest, openapiCustomBatchResponse]("/api/custom-batch", pathItem)
			},
			operation:  func(pathItem *openapi3.PathItem) *openapi3.Operation { return pathItem.Post },
			wantStatus: "200",
		},
		{
			name: "delete many",
			set: func(pathItem *openapi3.PathItem) {
				setDeleteMany[*openapiCustomBatchModel, openapiCustomBatchRequest, openapiCustomBatchResponse]("/api/custom-batch", pathItem)
			},
			operation:  func(pathItem *openapi3.PathItem) *openapi3.Operation { return pathItem.Delete },
			wantStatus: "200",
		},
		{
			name: "update many",
			set: func(pathItem *openapi3.PathItem) {
				setUpdateMany[*openapiCustomBatchModel, openapiCustomBatchRequest, openapiCustomBatchResponse]("/api/custom-batch", pathItem)
			},
			operation:  func(pathItem *openapi3.PathItem) *openapi3.Operation { return pathItem.Put },
			wantStatus: "200",
		},
		{
			name: "patch many",
			set: func(pathItem *openapi3.PathItem) {
				setPatchMany[*openapiCustomBatchModel, openapiCustomBatchRequest, openapiCustomBatchResponse]("/api/custom-batch", pathItem)
			},
			operation:  func(pathItem *openapi3.PathItem) *openapi3.Operation { return pathItem.Patch },
			wantStatus: "200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathItem := &openapi3.PathItem{}
			tt.set(pathItem)

			op := tt.operation(pathItem)
			if op == nil || op.Responses == nil || op.Responses.Value(tt.wantStatus) == nil {
				t.Fatalf("documented responses = %v, want status %s", op.Responses, tt.wantStatus)
			}
			if op.Responses.Value("201") != nil {
				t.Fatal("custom batch response unexpectedly includes status 201")
			}

			schema := registeredResponseSchema(t, op.Responses.Value(tt.wantStatus))
			for _, name := range []string{"code", "msg", "data", "trace_id"} {
				if schema.Properties[name] == nil {
					t.Errorf("response envelope property %q is missing", name)
				}
			}
			data := schema.Properties["data"]
			if data == nil || data.Value == nil || data.Value.Properties["result"] == nil {
				t.Fatalf("response data schema = %#v, want the custom response shape", data)
			}
		})
	}
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
	// GroupRoles binds groups to their roles.
	GroupRoles map[string][]string `json:"group_roles,omitempty"`
}

func TestOpenAPIDocCommentRemovesExactGoDocSubject(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		comment string
		want    string
	}{
		{
			name:    "Chinese copula",
			symbol:  "Label",
			comment: "Label 是字段标签。",
			want:    "字段标签。",
		},
		{
			name:    "Chinese question phrase",
			symbol:  "Enabled",
			comment: "Enabled 是否启用。",
			want:    "是否启用。",
		},
		{
			name:    "English copula",
			symbol:  "Name",
			comment: "Name is the option name.",
			want:    "The option name.",
		},
		{
			name:    "English plural copula",
			symbol:  "Options",
			comment: "Options are available choices.",
			want:    "Available choices.",
		},
		{
			name:    "tab symbol boundary",
			symbol:  "Name",
			comment: "Name\tis the display name.",
			want:    "The display name.",
		},
		{
			name:    "wrapped English copula",
			symbol:  "Name",
			comment: "Name is\nthe display name.",
			want:    "The display name.",
		},
		{
			name:    "English verb",
			symbol:  "GroupRoles",
			comment: "GroupRoles binds groups to their roles.",
			want:    "Binds groups to their roles.",
		},
		{
			name:    "ASCII colon",
			symbol:  "Name",
			comment: "Name: display name.",
			want:    "Display name.",
		},
		{
			name:    "full width colon",
			symbol:  "Name",
			comment: "Name：显示名称。",
			want:    "显示名称。",
		},
		{
			name:    "multiple lines",
			symbol:  "Model",
			comment: "Model is the model summary.\n\nMore details.",
			want:    "The model summary.\n\nMore details.",
		},
		{
			name:    "similar identifier",
			symbol:  "ID",
			comment: "IDCard is the identity card.",
			want:    "IDCard is the identity card.",
		},
		{
			name:    "comment without subject",
			symbol:  "Name",
			comment: "显示名称。",
			want:    "显示名称。",
		},
		{
			name:    "subject only",
			symbol:  "Name",
			comment: "Name",
			want:    "Name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := openAPIDocComment(tt.symbol, tt.comment); got != tt.want {
				t.Fatalf("openAPIDocComment(%q, %q) = %q, want %q", tt.symbol, tt.comment, got, tt.want)
			}
		})
	}
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

type jsonDocPayload struct {
	// Code is the payload code.
	Code string `json:"code"`
}

type jsonDocModel struct {
	// Config is the configuration payload.
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
	if title.Value.Title != "" {
		t.Fatalf("title schema title = %q, want empty", title.Value.Title)
	}
	if title.Value.Description != "The category title." {
		t.Fatalf("title description = %q, want API-facing field comment", title.Value.Description)
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
	types := map[string]reflect.Type{
		"value":             reflect.TypeFor[summaryFirstLineModel](),
		"pointer":           reflect.TypeFor[*summaryFirstLineModel](),
		"slice":             reflect.TypeFor[[]summaryFirstLineModel](),
		"slice of pointers": reflect.TypeFor[[]*summaryFirstLineModel](),
	}

	for name, typ := range types {
		t.Run(name, func(t *testing.T) {
			got := summary("/api/play/customizations", consts.Patch, typ)
			if got != "The human readable summary line." {
				t.Fatalf("summary() = %q, want the first comment line", got)
			}
		})
	}
}

func TestDescriptionRemovesStructNameAndKeepsRemainingLines(t *testing.T) {
	want := "The human readable summary line.\nThe second comment line must not leak into the summary."
	types := map[string]reflect.Type{
		"value":             reflect.TypeFor[summaryFirstLineModel](),
		"pointer":           reflect.TypeFor[*summaryFirstLineModel](),
		"slice":             reflect.TypeFor[[]summaryFirstLineModel](),
		"slice of pointers": reflect.TypeFor[[]*summaryFirstLineModel](),
	}

	for name, typ := range types {
		t.Run(name, func(t *testing.T) {
			got := description(consts.Patch, typ)
			if got != want {
				t.Fatalf("description() = %q, want API-facing full struct comment", got)
			}
		})
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
