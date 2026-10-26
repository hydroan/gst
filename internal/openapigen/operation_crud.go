package openapigen

import (
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/types"
)

func setCreate[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, consts.Create)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, consts.Create)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	if !custom {
		hideUnwrittenFields(reqSchemaRef, typ, consts.Create)
	}
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, consts.Create, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, consts.Create, custom)
	pathItem.Post = op
}

func setDelete[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, consts.Delete)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, consts.Delete)
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	// The framework's own delete answers without data; only a service's
	// delete answers with its response type.
	if !custom {
		markEmptyResponseData(rspSchemaRef)
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	op := newOperation(path, consts.Delete, typ, custom)
	op.Responses = newResponses(rspKey, consts.Delete, custom)
	pathItem.Delete = op
}

func setUpdate[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, consts.Update)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, consts.Update)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	if !custom {
		hideUnwrittenFields(reqSchemaRef, typ, consts.Update)
	}
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, consts.Update, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, consts.Update, custom)
	pathItem.Put = op
}

func setPatch[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, consts.Patch)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, consts.Patch)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	if !custom {
		hideUnwrittenFields(reqSchemaRef, typ, consts.Patch)
	}
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, consts.Patch, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, consts.Patch, custom)
	pathItem.Patch = op
}

func setList[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, consts.List)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, consts.List)

	var rspSchemaRef *openapi3.SchemaRef
	if !custom {
		rspSchemaRef = newSchemaRefWithDocs(apiListResponse[M]{})
	} else {
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	op := newOperation(path, consts.List, typ, custom)
	op.Responses = newResponses(rspKey, consts.List, custom)
	pathItem.Get = op
	addQueryParameters[M, REQ, RSP](pathItem.Get, true)
}

func setGet[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, consts.Get)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, consts.Get)
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	op := newOperation(path, consts.Get, typ, custom)
	op.Responses = newResponses(rspKey, consts.Get, custom)
	pathItem.Get = op
}

func setCreateMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, consts.CreateMany)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, consts.CreateMany)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if !custom {
		reqSchemaRef = newSchemaRefWithDocs(apiBatchRequest[REQ]{})
		hideUnwrittenFields(reqSchemaRef, typ, consts.CreateMany)
		rspSchemaRef = newSchemaRefWithDocs(apiBatchResponse[RSP]{})
	} else {
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, consts.CreateMany, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, consts.CreateMany, custom)
	pathItem.Post = op
}

func setDeleteMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, consts.DeleteMany)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, consts.DeleteMany)
	reqSchemaRef := deleteManyIDsRequestSchema()
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	// The framework's own batch delete answers without data, like the single
	// delete; only a service's batch delete answers with its response type.
	if !custom {
		markEmptyResponseData(rspSchemaRef)
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, consts.DeleteMany, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, consts.DeleteMany, custom)
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
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, consts.UpdateMany)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, consts.UpdateMany)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if !custom {
		reqSchemaRef = newSchemaRefWithDocs(apiBatchRequest[REQ]{})
		hideUnwrittenFields(reqSchemaRef, typ, consts.UpdateMany)
		rspSchemaRef = newSchemaRefWithDocs(apiBatchResponse[RSP]{})
	} else {
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, consts.UpdateMany, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, consts.UpdateMany, custom)
	pathItem.Put = op
}

func setPatchMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeFor[M]()
	custom := !modelregistry.AreTypesEqual[M, REQ, RSP]()
	reqKey := actionComponentKey(reflect.TypeFor[REQ](), typ, path, consts.PatchMany)
	rspKey := actionComponentKey(reflect.TypeFor[RSP](), typ, path, consts.PatchMany)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if !custom {
		reqSchemaRef = newSchemaRefWithDocs(apiBatchRequest[REQ]{})
		hideUnwrittenFields(reqSchemaRef, typ, consts.PatchMany)
		rspSchemaRef = newSchemaRefWithDocs(apiBatchResponse[RSP]{})
	} else {
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	op := newOperation(path, consts.PatchMany, typ, custom)
	op.RequestBody = newRequestBody[REQ](reqKey)
	op.Responses = newResponses(rspKey, consts.PatchMany, custom)
	pathItem.Patch = op
}
