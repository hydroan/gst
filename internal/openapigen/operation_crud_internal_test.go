package openapigen

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelregistry"
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
