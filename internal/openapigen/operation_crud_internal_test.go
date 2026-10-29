package openapigen

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/tenant"
	"github.com/stretchr/testify/require"
)

type openapiDefaultCreateModel struct {
	Name string `json:"name"`

	modelregistry.Base
}

type openapiCustomCreateModel struct {
	Name string `json:"name"`

	modelregistry.Base
}

type openapiCustomCreateRequest struct {
	Name string `json:"name"`
}

type openapiCustomCreateResponse struct {
	Result string `json:"result"`
}

// TestSetCreateDocumentsSuccessStatus guards the documented success status of
// create. The default and the custom form both answer 200 at runtime, so
// neither may be documented as created.
func TestSetCreateDocumentsSuccessStatus(t *testing.T) {
	tests := []struct {
		name string
		set  func(*openapi3.PathItem)
	}{
		{
			name: "default create",
			set: func(pathItem *openapi3.PathItem) {
				setCreate[*openapiDefaultCreateModel, *openapiDefaultCreateModel, *openapiDefaultCreateModel]("/api/default-create", pathItem)
			},
		},
		{
			name: "custom create",
			set: func(pathItem *openapi3.PathItem) {
				setCreate[*openapiCustomCreateModel, openapiCustomCreateRequest, openapiCustomCreateResponse]("/api/custom-create", pathItem)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathItem := &openapi3.PathItem{}
			tt.set(pathItem)

			if pathItem.Post == nil || pathItem.Post.Responses == nil || pathItem.Post.Responses.Value("200") == nil {
				t.Fatalf("documented responses = %v, want status 200", pathItem.Post.Responses)
			}
			if pathItem.Post.Responses.Value("201") != nil {
				t.Fatal("documented responses unexpectedly include status 201")
			}
		})
	}
}

// TestOperationsDocumentTheFailuresTheyAnswer pins the failure responses of
// every action next to the 200 of its success: the statuses the controller
// answers a client's own failure with, by action, and a default response for
// every other failure, all referring to the one failure envelope component,
// whose data is null.
func TestOperationsDocumentTheFailuresTheyAnswer(t *testing.T) {
	type model = openapiDefaultCreateModel
	tests := []struct {
		name     string
		set      func(*openapi3.PathItem) *openapi3.Operation
		statuses []string
	}{
		{
			name: "default create",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setCreate[*model, *model, *model]("/api/failures/create", p)
				return p.Post
			},
			statuses: []string{"200", "400", "409", "default"},
		},
		{
			name: "custom create",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setCreate[*openapiCustomCreateModel, openapiCustomCreateRequest, openapiCustomCreateResponse]("/api/failures/custom-create", p)
				return p.Post
			},
			statuses: []string{"200", "400", "default"},
		},
		{
			name: "default get",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setGet[*model, *model, *model]("/api/failures/get/{id}", p)
				return p.Get
			},
			statuses: []string{"200", "400", "404", "default"},
		},
		{
			name: "default list",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setList[*model, *model, *model]("/api/failures/list", p)
				return p.Get
			},
			statuses: []string{"200", "400", "default"},
		},
		{
			name: "default update",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setUpdate[*model, *model, *model]("/api/failures/update/{id}", p)
				return p.Put
			},
			statuses: []string{"200", "400", "404", "409", "default"},
		},
		{
			name: "default patch",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setPatch[*model, *model, *model]("/api/failures/patch/{id}", p)
				return p.Patch
			},
			statuses: []string{"200", "400", "404", "409", "default"},
		},
		{
			name: "default delete",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setDelete[*model, *model, *model]("/api/failures/delete/{id}", p)
				return p.Delete
			},
			statuses: []string{"200", "400", "404", "409", "default"},
		},
		{
			name: "default create many",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setCreateMany[*model, *model, *model]("/api/failures/create-many", p)
				return p.Post
			},
			statuses: []string{"200", "400", "409", "default"},
		},
		{
			name: "default update many",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setUpdateMany[*model, *model, *model]("/api/failures/update-many", p)
				return p.Put
			},
			statuses: []string{"200", "400", "404", "409", "default"},
		},
		{
			name: "default patch many",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setPatchMany[*model, *model, *model]("/api/failures/patch-many", p)
				return p.Patch
			},
			statuses: []string{"200", "400", "404", "409", "default"},
		},
		{
			name: "default delete many",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setDeleteMany[*model, *model, *model]("/api/failures/delete-many", p)
				return p.Delete
			},
			statuses: []string{"200", "400", "404", "409", "default"},
		},
		{
			name: "import",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setImport[*model, *model, *model]("/api/failures/import", p)
				return p.Post
			},
			statuses: []string{"200", "400", "404", "409", "default"},
		},
		{
			name: "export",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setExport[*model, *model, *model]("/api/failures/export", p)
				return p.Get
			},
			statuses: []string{"200", "400", "default"},
		},
		{
			name: "sse",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setSSE[*model, *model, *model]("/api/failures/sse", p)
				return p.Get
			},
			statuses: []string{"200", "400", "default"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := tt.set(&openapi3.PathItem{})
			statuses := slices.Sorted(maps.Keys(op.Responses.Map()))
			require.Equal(t, tt.statuses, statuses)

			failure := op.Responses.Default()
			require.NotNil(t, failure)
			require.Equal(t, "#/components/responses/"+failureResponseKey, failure.Ref)
			for _, status := range tt.statuses[1 : len(tt.statuses)-1] {
				require.Equal(t, failure.Ref, op.Responses.Value(status).Ref, "status %s", status)
			}
			envelope := registeredResponseSchema(t, failure)
			require.ElementsMatch(t, []string{"data", "msg", "trace_id"}, propertyNames(envelope))
			data := dataSchema(t, envelope)
			require.True(t, data.Nullable, "a failure answers data null")
			require.Nil(t, data.Type)
		})
	}
}

// TestResponsesOfAnswersItsExample guards the example of responsesOf: the
// create of a Record in the model root package, which the framework handles
// itself, with success referring to its response component record_create,
// answers 200 with that component and every failure with the failure envelope.
func TestResponsesOfAnswersItsExample(t *testing.T) {
	responses := responsesOf(&openapi3.ResponseRef{Ref: "#/components/responses/record_create"}, consts.Create, false)

	refs := make(map[string]string)
	for status, ref := range responses.Map() {
		refs[status] = ref.Ref
	}
	require.Equal(t, map[string]string{
		"200":     "#/components/responses/record_create",
		"400":     "#/components/responses/Failure",
		"409":     "#/components/responses/Failure",
		"default": "#/components/responses/Failure",
	}, refs)
}

type openapiCustomBatchModel struct {
	Name string `json:"name"`

	modelregistry.Base
}

type openapiCustomBatchRequest struct {
	Name string `json:"name"`
}

type openapiCustomBatchResponse struct {
	Result string `json:"result"`
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
			for _, name := range []string{"msg", "data", "trace_id"} {
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

// openapiWriteModel declares a column of each gorm write permission beside
// the framework base, whose fields the framework writes itself.
type openapiWriteModel struct {
	Name  string `json:"name"`
	Code  string `json:"code" gorm:"<-:create"`
	Stamp string `json:"stamp" gorm:"<-:update"`
	Mark  string `json:"mark" gorm:"->"`

	modelregistry.Base
}

// openapiOwnKeyModel declares a primary key of its own over the framework
// base's, the way a model widens its id: the key is still the one a route or
// a batch item names the record by.
type openapiOwnKeyModel struct {
	ID   string `json:"id" gorm:"primaryKey;size:191"`
	Name string `json:"name"`

	modelregistry.Base
}

// openapiRequiredModel requires Name and Code by its binding rules, Code
// written on insert alone, beside a Note no rule requires and Tags, whose
// elements the rules after dive require rather than the field itself.
type openapiRequiredModel struct {
	Name string   `json:"name" binding:"required"`
	Code string   `json:"code" gorm:"<-:create" binding:"required,max=8"`
	Note string   `json:"note"`
	Tags []string `json:"tags" gorm:"serializer:json" binding:"dive,required"`

	modelregistry.Base
}

// TestSetMarksTheFieldsABindingRequires pins that a field the binding rules
// require, which a request without it is refused for, is a required property
// of the body documented, and that a field the body leaves out is required
// by none. A patch checks the fields it names alone, so it requires none of
// them, an item of a batch patch requiring its id alone.
func TestSetMarksTheFieldsABindingRequires(t *testing.T) {
	create := &openapi3.PathItem{}
	setCreate[*openapiRequiredModel, *openapiRequiredModel, *openapiRequiredModel]("/api/required/create", create)
	require.ElementsMatch(t, []string{"code", "name"}, registeredRequestBodySchema(t, create.Post.RequestBody).Required)

	update := &openapi3.PathItem{}
	setUpdate[*openapiRequiredModel, *openapiRequiredModel, *openapiRequiredModel]("/api/required/update/{id}", update)
	require.ElementsMatch(t, []string{"name"}, registeredRequestBodySchema(t, update.Put.RequestBody).Required)

	patch := &openapi3.PathItem{}
	setPatch[*openapiRequiredModel, *openapiRequiredModel, *openapiRequiredModel]("/api/required/patch/{id}", patch)
	require.Empty(t, registeredRequestBodySchema(t, patch.Patch.RequestBody).Required)

	patchMany := &openapi3.PathItem{}
	setPatchMany[*openapiRequiredModel, *openapiRequiredModel, *openapiRequiredModel]("/api/required/patch-many", patchMany)
	item := batchItemSchema(registeredRequestBodySchema(t, patchMany.Patch.RequestBody))
	require.NotNil(t, item)
	require.ElementsMatch(t, []string{"id"}, item.Required)
}

// openapiScopedModel's records each belong to one tenant.
type openapiScopedModel struct {
	Name string `json:"name"`

	tenant.Scope
	modelregistry.Base
}

// TestSetDocumentsWhoNamesTheTenant pins that the tenant_id a create body
// documents says who sets it: the record is filed under the caller's tenant,
// a caller acting across tenants alone naming another.
func TestSetDocumentsWhoNamesTheTenant(t *testing.T) {
	create := &openapi3.PathItem{}
	setCreate[*openapiScopedModel, *openapiScopedModel, *openapiScopedModel]("/api/scoped/create", create)
	property := registeredRequestBodySchema(t, create.Post.RequestBody).Properties["tenant_id"]
	require.NotNil(t, property)
	require.Contains(t, property.Value.Description, "acts across tenants")
}

// openapiWriteRequest is a custom payload whose fields carry the names of
// the framework base's: they are the service's own to read.
type openapiWriteRequest struct {
	ID        string `json:"id"`
	CreatedBy string `json:"created_by"`
	Code      string `json:"code"`
}

// TestSetDocumentsTheFieldsAWriteTakes pins the request body of each write to
// the fields it takes from the client, and its example to the same fields.
// The framework's own flow takes none of the framework base's, which it writes
// itself, but the primary key a create may name, the insert generating one
// otherwise, and no column gorm leaves out of the write: an insert none
// written on update only or never, an update none written on insert only or
// never. An item of a batch update names its record by its id, which it must
// carry, not optional as on a create. A custom payload is the service's to
// read, every field of it, a batch's included, which is the payload itself
// rather than a list of items.
func TestSetDocumentsTheFieldsAWriteTakes(t *testing.T) {
	type model = openapiWriteModel
	tests := []struct {
		name     string
		set      func(*openapi3.PathItem) *openapi3.Operation
		batch    bool
		fields   []string
		required []string
	}{
		{
			name: "create",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setCreate[*model, *model, *model]("/api/writes/create", p)
				return p.Post
			},
			fields: []string{"code", "id", "name"},
		},
		{
			name: "update",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setUpdate[*model, *model, *model]("/api/writes/update/{id}", p)
				return p.Put
			},
			fields: []string{"name", "stamp"},
		},
		{
			name: "patch",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setPatch[*model, *model, *model]("/api/writes/patch/{id}", p)
				return p.Patch
			},
			fields: []string{"name", "stamp"},
		},
		{
			name: "create many",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setCreateMany[*model, *model, *model]("/api/writes/create-many", p)
				return p.Post
			},
			batch:  true,
			fields: []string{"code", "id", "name"},
		},
		{
			name: "update many",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setUpdateMany[*model, *model, *model]("/api/writes/update-many", p)
				return p.Put
			},
			batch:    true,
			fields:   []string{"id", "name", "stamp"},
			required: []string{"id"},
		},
		{
			name: "patch many",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setPatchMany[*model, *model, *model]("/api/writes/patch-many", p)
				return p.Patch
			},
			batch:    true,
			fields:   []string{"id", "name", "stamp"},
			required: []string{"id"},
		},
		{
			name: "custom create",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setCreate[*model, openapiWriteRequest, openapiWriteRequest]("/api/writes/custom-create", p)
				return p.Post
			},
			fields: []string{"code", "created_by", "id"},
		},
		{
			name: "custom update",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setUpdate[*model, openapiWriteRequest, openapiWriteRequest]("/api/writes/custom-update/{id}", p)
				return p.Put
			},
			fields: []string{"code", "created_by", "id"},
		},
		{
			name: "custom patch",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setPatch[*model, openapiWriteRequest, openapiWriteRequest]("/api/writes/custom-patch/{id}", p)
				return p.Patch
			},
			fields: []string{"code", "created_by", "id"},
		},
		{
			name: "custom create many",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setCreateMany[*model, openapiWriteRequest, openapiWriteRequest]("/api/writes/custom-create-many", p)
				return p.Post
			},
			fields: []string{"code", "created_by", "id"},
		},
		{
			name: "custom update many",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setUpdateMany[*model, openapiWriteRequest, openapiWriteRequest]("/api/writes/custom-update-many", p)
				return p.Put
			},
			fields: []string{"code", "created_by", "id"},
		},
		{
			name: "custom patch many",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setPatchMany[*model, openapiWriteRequest, openapiWriteRequest]("/api/writes/custom-patch-many", p)
				return p.Patch
			},
			fields: []string{"code", "created_by", "id"},
		},
		{
			name: "create with a key of its own",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setCreate[*openapiOwnKeyModel, *openapiOwnKeyModel, *openapiOwnKeyModel]("/api/writes/own-key", p)
				return p.Post
			},
			fields: []string{"id", "name"},
		},
		{
			name: "update with a key of its own",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setUpdate[*openapiOwnKeyModel, *openapiOwnKeyModel, *openapiOwnKeyModel]("/api/writes/own-key/{id}", p)
				return p.Put
			},
			fields: []string{"name"},
		},
		{
			name: "update many with a key of its own",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setUpdateMany[*openapiOwnKeyModel, *openapiOwnKeyModel, *openapiOwnKeyModel]("/api/writes/own-key/many", p)
				return p.Put
			},
			batch:    true,
			fields:   []string{"id", "name"},
			required: []string{"id"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := registeredRequestBodySchema(t, tt.set(&openapi3.PathItem{}).RequestBody)
			exampleJSON, err := json.Marshal(body.Example)
			require.NoError(t, err)
			var example map[string]any
			if tt.batch {
				var batch struct {
					Items []map[string]any `json:"items"`
				}
				require.NoError(t, json.Unmarshal(exampleJSON, &batch))
				require.Len(t, batch.Items, 1)
				example = batch.Items[0]
				body = batchItemSchema(body)
				require.NotNil(t, body)
			} else {
				require.NoError(t, json.Unmarshal(exampleJSON, &example))
			}

			require.Equal(t, tt.fields, propertyNames(body))
			require.Equal(t, tt.fields, slices.Sorted(maps.Keys(example)))
			require.ElementsMatch(t, tt.required, body.Required)
		})
	}
}
