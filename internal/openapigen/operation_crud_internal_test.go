package openapigen

import (
	"maps"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelregistry"
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
			statuses: []string{"200", "400", "409", "default"},
		},
		{
			name: "import",
			set: func(p *openapi3.PathItem) *openapi3.Operation {
				setImport[*model, *model, *model]("/api/failures/import", p)
				return p.Post
			},
			statuses: []string{"200", "400", "409", "default"},
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
