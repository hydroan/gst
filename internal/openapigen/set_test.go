package openapigen

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
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
	// Page is a business filter field; the bare name no longer collides with
	// the framework pagination parameter, which lives in the "_" namespace.
	Page string `json:"-" query:"page"`

	model.Query
	model.Base
}

type openapiUnsafeQueryModel struct {
	model.Query
	model.UnsafeQuery
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

type openapiExportModel struct {
	// Name is the sample name.
	Name string `json:"name" query:"name"`

	model.Base
}

type openapiImportModel struct {
	// Name is the sample name.
	Name string `json:"name"`

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
				"page",
				"_page", "_size",
				"_cursor_value", "_cursor_field", "_cursor_next",
				"_expand", "_depth", "_sort_by",
				"id", "created_by", "updated_by",
				"created_at[op]", "updated_at[op]",
			},
		},
		{
			name: "unsafe query",
			add: func(op *openapi3.Operation) {
				addQueryParameters[*openapiUnsafeQueryModel, *openapiUnsafeQueryModel, *openapiUnsafeQueryModel](op)
			},
			parameters: []string{
				"_page", "_size",
				"_cursor_value", "_cursor_field", "_cursor_next",
				"_expand", "_depth", "_sort_by",
				"_or", "_index", "_select", "_no_cache", "_no_total",
				"id", "created_by", "updated_by",
				"created_at[op]", "updated_at[op]",
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
		"created_at[op]", "updated_at[op]",
		"_page", "_size", "_sort_by",
		"_expand", "_depth",
		"_cursor_value", "_cursor_field", "_cursor_next",
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

	model.Query
	model.Base
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
		t.Fatalf("id description = %q, models without model.Query must not advertise operator filters", parameters["id"].Description)
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

func TestSetExportDocumentsFileDownload(t *testing.T) {
	pathItem := &openapi3.PathItem{}
	setExport[*openapiExportModel, *openapiExportModel, *openapiExportModel]("/api/exports/export", pathItem)

	op := pathItem.Get
	if op == nil {
		t.Fatal("setExport did not document a GET operation")
	}
	if op.OperationID == "" || op.Summary == "" {
		t.Fatalf("export operation id/summary missing: id=%q summary=%q", op.OperationID, op.Summary)
	}
	if len(op.Tags) == 0 || op.Tags[0] != "exports" {
		t.Fatalf("export tags = %v, want [exports]", op.Tags)
	}

	// The 200 response advertises binary file downloads instead of a JSON envelope.
	if op.Responses == nil || op.Responses.Value("200") == nil || op.Responses.Value("200").Value == nil {
		t.Fatalf("export responses = %v, want an inline status 200", op.Responses)
	}
	content := op.Responses.Value("200").Value.Content
	for _, mediaType := range []string{"text/csv", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"} {
		mt := content[mediaType]
		if mt == nil || mt.Schema == nil || mt.Schema.Value == nil || mt.Schema.Value.Format != "binary" {
			t.Fatalf("export 200 media type %q is not a binary file schema: %#v", mediaType, mt)
		}
	}

	// The file-format query parameter documents the supported formats.
	params := queryParametersByName(t, op)
	format := params[consts.QUERY_FORMAT]
	if format == nil || format.Schema == nil || format.Schema.Value == nil {
		t.Fatalf("export query parameters = %v, want %q", parameterNames(params), consts.QUERY_FORMAT)
	}
	if got := format.Schema.Value.Enum; !reflect.DeepEqual(got, []any{"csv", "xlsx"}) {
		t.Fatalf("export %q enum = %v, want [csv xlsx]", consts.QUERY_FORMAT, got)
	}
	// Export shares the list filters, so model field queries are documented too.
	if params["name"] == nil {
		t.Fatalf("export query parameters = %v, want model filter %q", parameterNames(params), "name")
	}
}

func TestSetImportDocumentsMultipartUpload(t *testing.T) {
	pathItem := &openapi3.PathItem{}
	setImport[*openapiImportModel, *openapiImportModel, *openapiImportModel]("/api/imports/import", pathItem)

	op := pathItem.Post
	if op == nil {
		t.Fatal("setImport did not document a POST operation")
	}
	if op.OperationID == "" || op.Summary == "" {
		t.Fatalf("import operation id/summary missing: id=%q summary=%q", op.OperationID, op.Summary)
	}
	if len(op.Tags) == 0 || op.Tags[0] != "imports" {
		t.Fatalf("import tags = %v, want [imports]", op.Tags)
	}

	// The request body is a required multipart/form-data upload with a binary "file" field.
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		t.Fatal("import operation is missing a request body")
	}
	body := op.RequestBody.Value
	if !body.Required {
		t.Fatal("import request body should be required")
	}
	mediaType := body.Content["multipart/form-data"]
	if mediaType == nil || mediaType.Schema == nil || mediaType.Schema.Value == nil {
		t.Fatalf("import request body content = %v, want multipart/form-data", body.Content)
	}
	fileSchema := mediaType.Schema.Value.Properties["file"]
	if fileSchema == nil || fileSchema.Value == nil || fileSchema.Value.Format != "binary" {
		t.Fatalf("import request body file property = %#v, want a binary schema", fileSchema)
	}
	if !slices.Contains(mediaType.Schema.Value.Required, "file") {
		t.Fatalf("import request body required = %v, want to include \"file\"", mediaType.Schema.Value.Required)
	}

	// The success envelope is documented as a 200 response.
	if op.Responses == nil || op.Responses.Value("200") == nil {
		t.Fatalf("import responses = %v, want status 200", op.Responses)
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

func TestSetupExampleKeepsNestedIDFields(t *testing.T) {
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

	setupExample(schemaRef)

	example, ok := schemaRef.Value.Example.(map[string]any)
	if !ok {
		t.Fatalf("example type = %T, want map[string]any", schemaRef.Value.Example)
	}

	// The top-level id is a Base auto field and stays removed from the example.
	if _, exists := example["id"]; exists {
		t.Fatalf("top-level id must be removed, got example = %#v", example)
	}

	// A nested struct's id is caller-supplied and must be kept.
	wantSingle := map[string]any{"id": "string", "name": "string"}
	if got := example["single"]; !reflect.DeepEqual(got, wantSingle) {
		t.Fatalf("single example = %#v, want %#v", got, wantSingle)
	}

	wantItems := []any{map[string]any{"id": "string", "name": "string"}}
	if got := example["items"]; !reflect.DeepEqual(got, wantItems) {
		t.Fatalf("items example = %#v, want %#v", got, wantItems)
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

	schemaRef, err := openapi3gen.NewSchemaRefForValue(*new(anonSchemaPayload), nil)
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

func TestSummaryCombinesVerbAndStructCommentFirstLine(t *testing.T) {
	types := map[string]reflect.Type{
		"value":             reflect.TypeFor[summaryFirstLineModel](),
		"pointer":           reflect.TypeFor[*summaryFirstLineModel](),
		"slice":             reflect.TypeFor[[]summaryFirstLineModel](),
		"slice of pointers": reflect.TypeFor[[]*summaryFirstLineModel](),
	}

	for name, typ := range types {
		t.Run(name, func(t *testing.T) {
			got := summary("/api/play/customizations", consts.Patch, typ, false)
			if got != "Patch The human readable summary line" {
				t.Fatalf("summary() = %q, want the verb plus the first comment line", got)
			}
		})
	}
}

func TestSummaryUsesTrailingActionSegmentForCustomTypes(t *testing.T) {
	typ := reflect.TypeFor[*summaryFirstLineModel]()
	got := summary("/api/users/{id}/disable", consts.Create, typ, true)
	if got != "Disable The human readable summary line" {
		t.Fatalf("summary() = %q, want the action segment plus the first comment line", got)
	}
}

func TestSummaryKeepsVerbForDefaultCRUDNestedCollection(t *testing.T) {
	typ := reflect.TypeFor[*summaryFirstLineModel]()
	got := summary("/api/tenants/{tenant}/users", consts.Create, typ, false)
	if got != "Create The human readable summary line" {
		t.Fatalf("summary() = %q, want the verb for a default CRUD nested collection", got)
	}
}

func TestSummaryAndDescriptionPreferRegisteredOperationDoc(t *testing.T) {
	apidoc.RegisterOperation("POST", "/api/override-users/:id/disable", apidoc.OperationDoc{
		Summary:     "The registered summary",
		Description: "The registered description.",
	})

	typ := reflect.TypeFor[*summaryFirstLineModel]()
	path := "/api/override-users/{id}/disable"
	if got := summary(path, consts.Create, typ, true); got != "The registered summary" {
		t.Fatalf("summary() = %q, want the registered override", got)
	}
	if got := description(path, consts.Create, typ, true); got != "The registered description." {
		t.Fatalf("description() = %q, want the registered override", got)
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
			got := description("/api/play/customizations", consts.Patch, typ, false)
			if got != want {
				t.Fatalf("description() = %q, want API-facing full struct comment", got)
			}
		})
	}
}

func TestSummaryFallsBackToPathSegments(t *testing.T) {
	typ := reflect.TypeOf(&struct{ Name string }{})
	got := summary("/api/play/customizations/{id}", consts.Patch, typ, false)
	if got != "Patch play customizations" {
		t.Fatalf("summary() = %q, want the path segment fallback", got)
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

// openapiActionModel backs two custom Create routes that carry distinct
// payloads, mirroring a resource that exposes several non-CRUD actions.
type openapiActionModel struct {
	// Name is the record name.
	Name string `json:"name"`

	model.Base
}

type openapiFirstActionReq struct {
	// Reason is the field unique to the first action.
	Reason string `json:"reason"`
}

type openapiFirstActionRsp struct {
	ID string `json:"id"`
}

type openapiSecondActionReq struct {
	// Token is the field unique to the second action.
	Token string `json:"token"`
}

type openapiSecondActionRsp struct {
	Name string `json:"name"`
}

// TestSetGivesEachCustomPayloadItsOwnRequestBody asserts that two actions
// sharing a model and a phase each keep their own payload, instead of
// collapsing onto one component keyed by the model.
func TestSetGivesEachCustomPayloadItsOwnRequestBody(t *testing.T) {
	Set[*openapiActionModel, *openapiFirstActionReq, *openapiFirstActionRsp]("/api/openapi-actions/first", true, consts.Create)
	Set[*openapiActionModel, *openapiSecondActionReq, *openapiSecondActionRsp]("/api/openapi-actions/second", true, consts.Create)

	first := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-actions/first").RequestBody)
	if _, ok := first.Properties["reason"]; !ok {
		t.Fatalf("first action payload properties = %v, want reason", propertyNames(first))
	}

	second := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-actions/second").RequestBody)
	if _, ok := second.Properties["token"]; !ok {
		t.Fatalf("second action payload properties = %v, want token", propertyNames(second))
	}
}

// TestSetGivesEachCustomResponseItsOwnComponent asserts the same isolation for
// responses, which share the request component key today.
func TestSetGivesEachCustomResponseItsOwnComponent(t *testing.T) {
	Set[*openapiActionModel, *openapiFirstActionReq, *openapiFirstActionRsp]("/api/openapi-responses/first", true, consts.Create)
	Set[*openapiActionModel, *openapiSecondActionReq, *openapiSecondActionRsp]("/api/openapi-responses/second", true, consts.Create)

	first := registeredResponseSchema(t, responseRefForPath(t, "/api/openapi-responses/first", 200))
	if _, ok := dataSchema(t, first).Properties["id"]; !ok {
		t.Fatalf("first action response data = %v, want id", propertyNames(dataSchema(t, first)))
	}

	second := registeredResponseSchema(t, responseRefForPath(t, "/api/openapi-responses/second", 200))
	if _, ok := dataSchema(t, second).Properties["name"]; !ok {
		t.Fatalf("second action response data = %v, want name", propertyNames(dataSchema(t, second)))
	}
}

type openapiNoDataReq struct {
	// Reason explains the action.
	Reason string `json:"reason"`
}

// openapiNoDataRsp carries no fields: the action answers with the plain
// envelope and a null data member.
type openapiNoDataRsp struct{}

// TestSetDeclaresResponsesForEmptyResponseType asserts that an action whose
// response type has no fields still documents its response. The handler answers
// {"code":0,"data":null,"msg":"success","trace_id":"..."}, and responses is a
// required member of an OpenAPI operation.
func TestSetDeclaresResponsesForEmptyResponseType(t *testing.T) {
	Set[*openapiActionModel, *openapiNoDataReq, *openapiNoDataRsp]("/api/openapi-nodata", true, consts.Create)

	op := operationForPath(t, "/api/openapi-nodata")
	if op.Responses == nil || op.Responses.Len() == 0 {
		t.Fatal("operation declares no responses")
	}
	schema := registeredResponseSchema(t, op.Responses.Status(200))
	for _, field := range []string{"code", "msg", "trace_id", "data"} {
		if _, ok := schema.Properties[field]; !ok {
			t.Errorf("response envelope = %v, want a %q member", propertyNames(schema), field)
		}
	}
}

// openapiTreeNode is self-referential, which makes the schema generator emit a
// component $ref to break the cycle instead of inlining forever.
type openapiTreeNode struct {
	// Label is the node label.
	Label string `json:"label"`

	// Children and Parent close the cycle.
	Children []*openapiTreeNode `json:"children,omitempty"`
	Parent   *openapiTreeNode   `json:"parent,omitempty"`

	model.Base
}

// TestSetResolvesSelfReferentialSchemaRefs asserts that the $ref emitted for a
// cyclic type points at a component that actually exists, so the document stays
// loadable.
func TestSetResolvesSelfReferentialSchemaRefs(t *testing.T) {
	Set[*openapiTreeNode, *openapiTreeNode, *openapiTreeNode]("/api/openapi-trees", true, consts.List)

	docMutex.RLock()
	schemas := doc.Components.Schemas
	docMutex.RUnlock()

	refs := collectSchemaRefs(t, schemas)
	if len(refs) == 0 {
		t.Fatal("no component $ref was emitted for the cyclic type, expected one to break the cycle")
	}
	for ref := range refs {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		if _, ok := schemas[name]; !ok {
			t.Errorf("$ref %q points at a component that is not registered; registered: %v", ref, schemaNames(schemas))
		}
	}
}

// openapiTreeHolder reaches the cyclic type through its own fields, the way a
// model holds a collection of tree-shaped records.
type openapiTreeHolder struct {
	// Nodes and Extra both reach openapiTreeNode, whose children close a cycle.
	Nodes []*openapiTreeNode `json:"nodes,omitempty"`
	Extra []*openapiTreeNode `json:"extra,omitempty"`

	model.Base
}

// TestSetBuildsExampleWithoutNullPlaceholders asserts that the generated
// example holds no null. A self-referential type recurses until the depth limit
// stops it, and stopping with a null puts an illegal value where the schema
// expects a non-nullable member, which fails document validation.
func TestSetBuildsExampleWithoutNullPlaceholders(t *testing.T) {
	Set[*openapiTreeHolder, *openapiTreeHolder, *openapiTreeHolder]("/api/openapi-tree-examples", true, consts.Create)

	schema := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-tree-examples").RequestBody)
	if schema.Example == nil {
		t.Fatal("request body schema carries no example")
	}
	if path, found := findNullInExample(schema.Example, ""); found {
		t.Fatalf("example carries null at %q, want a legal value for a non-nullable member", path)
	}
}

// openapiExampleShapeModel carries the field shapes whose example value is
// constrained by more than its JSON type.
type openapiExampleShapeModel struct {
	// Status is constrained to the registered enum values.
	Status enumFieldStatus `json:"status"`

	// Codes constrains its items to the registered enum values.
	Codes []enumFieldStatus `json:"codes"`

	// StartedAt is a date-time formatted string.
	StartedAt time.Time `json:"started_at"`

	model.Base
}

// TestSetBuildsExampleHonouringFormatAndEnum asserts that an example value
// respects the constraints its schema declares. A plain "string" placeholder
// fails validation where the schema declares a date-time format or an enum.
func TestSetBuildsExampleHonouringFormatAndEnum(t *testing.T) {
	registerEnumFieldStatus()
	Set[*openapiExampleShapeModel, *openapiExampleShapeModel, *openapiExampleShapeModel]("/api/openapi-example-shapes", true, consts.Create)

	schema := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-example-shapes").RequestBody)
	example, ok := schema.Example.(map[string]any)
	if !ok {
		t.Fatalf("example = %#v, want an object", schema.Example)
	}

	if got := example["status"]; got != "active" {
		t.Errorf("status example = %#v, want the first enum value %q", got, "active")
	}

	codes, ok := example["codes"].([]any)
	if !ok || len(codes) == 0 {
		t.Fatalf("codes example = %#v, want a populated array", example["codes"])
	}
	if codes[0] != "active" {
		t.Errorf("codes[0] example = %#v, want the first enum value %q", codes[0], "active")
	}

	startedAt, ok := example["started_at"].(string)
	if !ok {
		t.Fatalf("started_at example = %#v, want a string", example["started_at"])
	}
	if _, err := time.Parse(time.RFC3339, startedAt); err != nil {
		t.Errorf("started_at example = %q, want a date-time formatted value: %v", startedAt, err)
	}
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

	model.Base
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

// TestSetStopsExampleAtSchemaRefBoundaries asserts that the example does not
// expand a member the schema renders as a $ref. Such a member serializes as a
// bare $ref, so its inline Value is not the schema readers validate against,
// and walking into it invents values the referenced component rejects.
func TestSetStopsExampleAtSchemaRefBoundaries(t *testing.T) {
	Set[*openapiTreeHolder, *openapiTreeHolder, *openapiTreeHolder]("/api/openapi-ref-boundaries", true, consts.Create)

	schema := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-ref-boundaries").RequestBody)
	example, ok := schema.Example.(map[string]any)
	if !ok {
		t.Fatalf("example = %#v, want an object", schema.Example)
	}
	nodes, ok := example["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		t.Fatalf("nodes example = %#v, want a populated array", example["nodes"])
	}
	node, ok := nodes[0].(map[string]any)
	if !ok {
		t.Fatalf("nodes[0] example = %#v, want an object", nodes[0])
	}

	// children items and parent both resolve to the cyclic component.
	children, ok := node["children"].([]any)
	if !ok {
		t.Fatalf("children example = %#v, want an array", node["children"])
	}
	if len(children) != 0 {
		t.Errorf("children example = %#v, want an empty array because its items are a $ref", children)
	}
	if parent, ok := node["parent"]; ok {
		t.Errorf("parent example = %#v, want the $ref member left out", parent)
	}
}

// openapiKeyModel exercises every verb so each generated component key is
// covered.
type openapiKeyModel struct {
	// Name is the record name.
	Name string `json:"name"`

	model.Base
}

// componentKeyCharset is the character set the OpenAPI spec allows in a
// components key.
var componentKeyCharset = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// TestSetBuildsComponentKeysWithinSpecCharset asserts that every registered
// component key stays inside the character set the spec allows, across all
// verbs. A key outside it makes the whole document invalid.
func TestSetBuildsComponentKeysWithinSpecCharset(t *testing.T) {
	Set[*openapiKeyModel, *openapiKeyModel, *openapiKeyModel]("/api/openapi-keys", true,
		consts.Create, consts.Delete, consts.Update, consts.Patch, consts.List, consts.Get,
		consts.CreateMany, consts.DeleteMany, consts.UpdateMany, consts.PatchMany)

	docMutex.RLock()
	defer docMutex.RUnlock()

	for key := range doc.Components.RequestBodies {
		if !componentKeyCharset.MatchString(key) {
			t.Errorf("requestBodies key %q falls outside the spec charset %s", key, componentKeyCharset)
		}
	}
	for key := range doc.Components.Responses {
		if !componentKeyCharset.MatchString(key) {
			t.Errorf("responses key %q falls outside the spec charset %s", key, componentKeyCharset)
		}
	}
	for key := range doc.Components.Schemas {
		if !componentKeyCharset.MatchString(key) {
			t.Errorf("schemas key %q falls outside the spec charset %s", key, componentKeyCharset)
		}
	}
}

// openapiAnyMapModel holds a map whose keys and values are user-defined, so the
// value schema declares no type and constrains nothing.
type openapiAnyMapModel struct {
	// Attributes holds user-defined keys and values.
	Attributes map[string]any `json:"attributes"`

	model.Base
}

// TestSetBuildsExampleForUntypedMapValues asserts that a schema declaring no
// type still gets a non-null example. Such a schema accepts any value, but a
// null is only accepted when the schema is explicitly nullable.
func TestSetBuildsExampleForUntypedMapValues(t *testing.T) {
	Set[*openapiAnyMapModel, *openapiAnyMapModel, *openapiAnyMapModel]("/api/openapi-any-maps", true, consts.Create)

	schema := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-any-maps").RequestBody)
	example, ok := schema.Example.(map[string]any)
	if !ok {
		t.Fatalf("example = %#v, want an object", schema.Example)
	}
	if path, found := findNullInExample(example["attributes"], "/attributes"); found {
		t.Fatalf("example carries null at %q, want a value an untyped schema accepts", path)
	}
}

// findNullInExample reports the first path inside an example value that holds a
// null.
func findNullInExample(value any, path string) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return path, true
	case map[string]any:
		for key, member := range typed {
			if found, ok := findNullInExample(member, path+"/"+key); ok {
				return found, true
			}
		}
	case []any:
		for index, member := range typed {
			if found, ok := findNullInExample(member, fmt.Sprintf("%s/%d", path, index)); ok {
				return found, true
			}
		}
	}
	return "", false
}

func collectSchemaRefs(t *testing.T, schemas openapi3.Schemas) map[string]bool {
	t.Helper()

	found := map[string]bool{}
	var walk func(ref *openapi3.SchemaRef, depth int)
	walk = func(ref *openapi3.SchemaRef, depth int) {
		if ref == nil || depth > 10 {
			return
		}
		if ref.Ref != "" {
			found[ref.Ref] = true
			return
		}
		if ref.Value == nil {
			return
		}
		for _, property := range ref.Value.Properties {
			walk(property, depth+1)
		}
		walk(ref.Value.Items, depth+1)
		if ref.Value.AdditionalProperties.Schema != nil {
			walk(ref.Value.AdditionalProperties.Schema, depth+1)
		}
	}
	for _, ref := range schemas {
		walk(ref, 0)
	}
	return found
}

func schemaNames(schemas openapi3.Schemas) []string {
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type openapiFirstTwinReq struct {
	// Secret is shared with openapiSecondTwinReq, making the two structurally
	// identical while remaining distinct types.
	Secret string `json:"secret"`
}

type openapiSecondTwinReq struct {
	Secret string `json:"secret"`
}

type openapiTwinRsp struct {
	OK bool `json:"ok"`
}

// TestSetSeparatesStructurallyIdenticalPayloads asserts that two payload types
// with identical fields still resolve to their own components, since the key is
// derived from the type rather than from its shape.
func TestSetSeparatesStructurallyIdenticalPayloads(t *testing.T) {
	Set[*openapiActionModel, *openapiFirstTwinReq, *openapiTwinRsp]("/api/openapi-twins/first", true, consts.Create)
	Set[*openapiActionModel, *openapiSecondTwinReq, *openapiTwinRsp]("/api/openapi-twins/second", true, consts.Create)

	first := operationForPath(t, "/api/openapi-twins/first").RequestBody.Ref
	second := operationForPath(t, "/api/openapi-twins/second").RequestBody.Ref
	if first == second {
		t.Fatalf("both twin payloads resolved to %q, want one component each", first)
	}
}

// TestSetKeepsModelKeyForDefaultCRUD pins the component key of a default CRUD
// action, where the model doubles as request and response.
func TestSetKeepsModelKeyForDefaultCRUD(t *testing.T) {
	Set[*openapiActionModel, *openapiActionModel, *openapiActionModel]("/api/openapi-crud", true, consts.Create)

	op := operationForPath(t, "/api/openapi-crud")
	wantRef := "#/components/requestBodies/internal.openapigen.openapiactionmodel_create"
	if op.RequestBody.Ref != wantRef {
		t.Fatalf("default CRUD requestBody ref = %q, want %q", op.RequestBody.Ref, wantRef)
	}
}

// TestSetSeparatesListAndGetResponsesForSameType asserts that one response type
// reused across two phases keeps the list envelope apart from the single-item
// envelope.
func TestSetSeparatesListAndGetResponsesForSameType(t *testing.T) {
	Set[*openapiActionModel, *openapiActionModel, *openapiActionModel]("/api/openapi-envelope", true, consts.List)
	Set[*openapiActionModel, *openapiActionModel, *openapiActionModel]("/api/openapi-envelope/:id", true, consts.Get)

	list := dataSchema(t, registeredResponseSchema(t, responseRefForPath(t, "/api/openapi-envelope", 200)))
	if _, ok := list.Properties["items"]; !ok {
		t.Fatalf("list response data = %v, want items", propertyNames(list))
	}

	get := dataSchema(t, registeredResponseSchema(t, responseRefForPath(t, "/api/openapi-envelope/{id}", 200)))
	if _, ok := get.Properties["items"]; ok {
		t.Fatalf("get response data = %v, want a single record rather than items", propertyNames(get))
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
