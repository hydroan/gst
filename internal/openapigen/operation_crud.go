package openapigen

import (
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/types"
)

func setCreate[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, apiaction.Create)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, apiaction.Create)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	if !custom {
		hideUnwrittenFields(reqSchemaRef, typ, apiaction.Create)
	}
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, apiaction.Create, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, apiaction.Create, custom)
	pathItem.Post = op
}

func setDelete[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, apiaction.Delete)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, apiaction.Delete)
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	// The framework's own delete answers without data; only a service's
	// delete answers with its response type.
	if !custom {
		markEmptyResponseData(rspSchemaRef)
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	op := newOperation(path, apiaction.Delete, typ, custom)
	op.Responses = newResponses(rspKey, apiaction.Delete, custom)
	pathItem.Delete = op
}

func setUpdate[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, apiaction.Update)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, apiaction.Update)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	if !custom {
		hideUnwrittenFields(reqSchemaRef, typ, apiaction.Update)
	}
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, apiaction.Update, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, apiaction.Update, custom)
	pathItem.Put = op
}

func setPatch[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, apiaction.Patch)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, apiaction.Patch)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	if !custom {
		hideUnwrittenFields(reqSchemaRef, typ, apiaction.Patch)
	}
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, apiaction.Patch, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, apiaction.Patch, custom)
	pathItem.Patch = op
}

func setList[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, apiaction.List)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, apiaction.List)

	var rspSchemaRef *openapi3.SchemaRef
	if !custom {
		rspSchemaRef = newSchemaRefWithDocs(apiListResponse[M]{})
	} else {
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	op := newOperation(path, apiaction.List, typ, custom)
	op.Responses = newResponses(rspKey, apiaction.List, custom)
	pathItem.Get = op
	addQueryParameters[M, REQ, RSP](pathItem.Get, true)
}

func setGet[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, apiaction.Get)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, apiaction.Get)
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	op := newOperation(path, apiaction.Get, typ, custom)
	op.Responses = newResponses(rspKey, apiaction.Get, custom)
	pathItem.Get = op
}

func setCreateMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, apiaction.CreateMany)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, apiaction.CreateMany)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if !custom {
		reqSchemaRef = newSchemaRefWithDocs(apiBatchRequest[REQ]{})
		hideUnwrittenFields(reqSchemaRef, typ, apiaction.CreateMany)
		rspSchemaRef = newSchemaRefWithDocs(apiBatchResponse[RSP]{})
	} else {
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, apiaction.CreateMany, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, apiaction.CreateMany, custom)
	pathItem.Post = op
}

func setDeleteMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, apiaction.DeleteMany)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, apiaction.DeleteMany)
	reqSchemaRef := deleteManyIDsRequestSchema()
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	// The framework's own batch delete answers without data, like the single
	// delete; only a service's batch delete answers with its response type.
	if !custom {
		markEmptyResponseData(rspSchemaRef)
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, apiaction.DeleteMany, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, apiaction.DeleteMany, custom)
	pathItem.Delete = op
}

// deleteManyIDsRequestSchema documents the batch delete payload: the controller
// reads the identifiers of the records to delete from a required "ids" array.
func deleteManyIDsRequestSchema() *openapi3.SchemaRef {
	return &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:     &openapi3.Types{openapi3.TypeObject},
			Required: []string{"ids"},
			Properties: map[string]*openapi3.SchemaRef{
				"ids": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{openapi3.TypeArray},
						Items: &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type:   &openapi3.Types{openapi3.TypeString},
								Format: idFormat,
							},
						},
					},
				},
			},
		},
	}
}

func setUpdateMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, apiaction.UpdateMany)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, apiaction.UpdateMany)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if !custom {
		reqSchemaRef = newSchemaRefWithDocs(apiBatchRequest[REQ]{})
		hideUnwrittenFields(reqSchemaRef, typ, apiaction.UpdateMany)
		rspSchemaRef = newSchemaRefWithDocs(apiBatchResponse[RSP]{})
	} else {
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, apiaction.UpdateMany, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, apiaction.UpdateMany, custom)
	pathItem.Put = op
}

func setPatchMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, apiaction.PatchMany)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, apiaction.PatchMany)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if !custom {
		reqSchemaRef = newSchemaRefWithDocs(apiBatchRequest[REQ]{})
		hideUnwrittenFields(reqSchemaRef, typ, apiaction.PatchMany)
		rspSchemaRef = newSchemaRefWithDocs(apiBatchResponse[RSP]{})
	} else {
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, apiaction.PatchMany, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, apiaction.PatchMany, custom)
	pathItem.Patch = op
}
