package openapigen

import (
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

func setCreate[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_CREATE)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_CREATE)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Post = &openapi3.Operation{
		OperationID: operationID(path, consts.Create),
		Summary:     summary(path, consts.Create, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Create, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.Create, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](rspKey),
	}
	removeBaseAutoFieldsFromRequestBody(pathItem.Post)
}

func setDelete[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_DELETE)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_DELETE)
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	pathItem.Delete = &openapi3.Operation{
		OperationID: operationID(path, consts.Delete),
		Summary:     summary(path, consts.Delete, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Delete, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.Delete, typ),
		Parameters:  parseParametersFromPath(path),
		Responses:   newResponses[RSP](rspKey),
	}
}

func setUpdate[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_UPDATE)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_UPDATE)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Put = &openapi3.Operation{
		OperationID: operationID(path, consts.Update),
		Summary:     summary(path, consts.Update, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Update, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.Update, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](rspKey),
	}
	removeBaseAutoFieldsFromRequestBody(pathItem.Put)
}

func setPatch[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_PATCH)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_PATCH)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Patch = &openapi3.Operation{
		OperationID: operationID(path, consts.Patch),
		Summary:     summary(path, consts.Patch, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Patch, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.Patch, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](rspKey),
	}
	removeBaseAutoFieldsFromRequestBody(pathItem.Patch)
}

func setList[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_LIST)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_LIST)

	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		rspSchemaRef = newSchemaRefWithDocs(apiListResponse[M]{})
	} else {
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	pathItem.Get = &openapi3.Operation{
		OperationID: operationID(path, consts.List),
		Summary:     summary(path, consts.List, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.List, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.List, typ),
		Parameters:  parseParametersFromPath(path),
		Responses:   newResponses[RSP](rspKey),
	}
	addQueryParameters[M, REQ, RSP](pathItem.Get)
}

func setGet[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_GET)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_GET)
	rspSchemaRef := newSchemaRefWithDocs(apiResponse[RSP]{})
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	pathItem.Get = &openapi3.Operation{
		OperationID: operationID(path, consts.Get),
		Summary:     summary(path, consts.Get, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Get, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.Get, typ),
		Parameters:  parseParametersFromPath(path),
		Responses:   newResponses[RSP](rspKey),
	}
}

func setCreateMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_CREATE_MANY)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_CREATE_MANY)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		reqSchemaRef = newSchemaRefWithDocs(apiBatchRequest[REQ]{})
		rspSchemaRef = newSchemaRefWithDocs(apiBatchResponse[RSP]{})
	} else {
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Post = &openapi3.Operation{
		OperationID: operationID(path, consts.CreateMany),
		Summary:     summary(path, consts.CreateMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.CreateMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.CreateMany, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](rspKey),
	}
	removeBaseAutoFieldsFromBatchRequestBody(pathItem.Post)
}

func setDeleteMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_DELETE_MANY)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_DELETE_MANY)
	reqSchemaRef := deleteManyIDsRequestSchema()
	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		rspSchemaRef = newSchemaRefWithDocs(apiBatchResponse[RSP]{})
	} else {
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Delete = &openapi3.Operation{
		OperationID: operationID(path, consts.DeleteMany),
		Summary:     summary(path, consts.DeleteMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.DeleteMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.DeleteMany, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](rspKey),
	}
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
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_UPDATE_MANY)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_UPDATE_MANY)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		reqSchemaRef = newSchemaRefWithDocs(apiBatchRequest[REQ]{})
		rspSchemaRef = newSchemaRefWithDocs(apiBatchResponse[RSP]{})
	} else {
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Put = &openapi3.Operation{
		OperationID: operationID(path, consts.UpdateMany),
		Summary:     summary(path, consts.UpdateMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.UpdateMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.UpdateMany, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](rspKey),
	}
	removeBaseAutoFieldsFromBatchRequestBody(pathItem.Put)
}

func setPatchMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_PATCH_MANY)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_PATCH_MANY)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		reqSchemaRef = newSchemaRefWithDocs(apiBatchRequest[REQ]{})
		rspSchemaRef = newSchemaRefWithDocs(apiBatchResponse[RSP]{})
	} else {
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(apiResponse[RSP]{})
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Patch = &openapi3.Operation{
		OperationID: operationID(path, consts.PatchMany),
		Summary:     summary(path, consts.PatchMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.PatchMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.PatchMany, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](rspKey),
	}
	removeBaseAutoFieldsFromBatchRequestBody(pathItem.Patch)
}
