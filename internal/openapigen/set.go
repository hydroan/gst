package openapigen

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gertd/go-pluralize"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

var pluralizeCli = pluralize.NewClient()

var idFormat = "" // eg: "uuid"

var timeType = reflect.TypeFor[time.Time]()

var removeFieldMap = map[string]bool{
	"id":         true,
	"created_at": true,
	"created_by": true,
	"updated_at": true,
	"updated_by": true,
	"deleted_at": true,
	"deleted_by": true,
}

func setCreate[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	name := typ.Elem().Name()
	reqKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_CREATE)
	rspKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_CREATE)
	reqSchemaRef, _ := openapi3gen.NewSchemaRefForValue(*new(REQ), nil)
	rspSchemaRef, _ := openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil)
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	// gen := openapi3gen.NewGenerator()
	// var reqSchemaRef *openapi3.SchemaRef
	// var err error
	// if !model.IsModelEmpty[REQ]() {
	// 	if reqSchemaRef, err = gen.NewSchemaRefForValue(*new(REQ), nil); err == nil {
	// 		setupExample(reqSchemaRef)
	// 		addSchemaTitleDesc[M](reqSchemaRef)
	// 	}
	// }

	pathItem.Post = &openapi3.Operation{
		OperationID: operationID(consts.Create, typ),
		Summary:     summary(path, consts.Create, typ),
		Description: description(consts.Create, typ),
		Tags:        tags(path, consts.Create, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](201, rspKey),
		// RequestBody: &openapi3.RequestBodyRef{Ref: "#/components/requestBodies/" + reqKey},
		// Responses:   openapi3.NewResponses(openapi3.WithStatus(201, &openapi3.ResponseRef{Ref: "#/components/responses/" + rspKey})),

		// Responses: func() *openapi3.Responses {
		// 	resp := openapi3.NewResponses()
		// 	// var schemaRef200 *openapi3.SchemaRef
		// 	// // var schemaRef400 *openapi3.SchemaRef
		// 	// var err error
		// 	//
		// 	// if schemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil); err == nil {
		// 	// 	// Add field descriptions to response data schema
		// 	// 	if schemaRef200.Value != nil && schemaRef200.Value.Properties != nil {
		// 	// 		if dataProperty, exists := schemaRef200.Value.Properties["data"]; exists {
		// 	// 			addSchemaTitleDesc[RSP](dataProperty)
		// 	// 		}
		// 	// 	}
		// 	// }
		//
		// 	resp.Set("201", &openapi3.ResponseRef{
		// 		Ref: "#/components/responses/" + rspKey,
		// 		// Value: &openapi3.Response{
		// 		// 	Description: util.ValueOf(fmt.Sprintf("%s created", name)),
		// 		// 	Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef200),
		// 		// },
		// 	})
		//
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// if schemaRef400, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil); err != nil {
		// 	// 	zap.S().Error(err)
		// 	// 	schemaRef400 = new(openapi3.SchemaRef)
		// 	// }
		// 	// resp.Set("400", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf("Invalid request"),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef400),
		// 	// 	},
		// 	// })
		//
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// resp.Set("401", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf("Unauthorized"),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(errorSchemaRef),
		// 	// 	},
		// 	// })
		// 	// resp.Set("409", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s already exists", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(errorSchemaRef),
		// 	// 	},
		// 	// })
		// 	// resp.Set("500", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf("Internal server error"),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(errorSchemaRef),
		// 	// 	},
		// 	// })
		// 	return resp
		// }(),
	}
	addHeaderParameters(pathItem.Post)
	removeFieldsFromRequestBody(pathItem.Post)
}

func setDelete[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	name := typ.Elem().Name()
	reqKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_DELETE)
	rspKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_DELETE)
	rspSchemaRef, _ := openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil)
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	pathItem.Delete = &openapi3.Operation{
		OperationID: operationID(consts.Delete, typ),
		Summary:     summary(path, consts.Delete, typ),
		Description: description(consts.Delete, typ),
		Tags:        tags(path, consts.Delete, typ),
		Parameters:  parseParametersFromPath(path),
		Responses:   newResponses[RSP](204, rspKey),
		// Responses: func() *openapi3.Responses {
		// 	var schemaRef204 *openapi3.SchemaRef
		// 	var err error
		// 	if schemaRef204, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil); err == nil {
		// 		// Add field descriptions to response data schema
		// 		if schemaRef204.Value != nil && schemaRef204.Value.Properties != nil {
		// 			if dataProperty, exists := schemaRef204.Value.Properties["data"]; exists {
		// 				addSchemaTitleDesc[RSP](dataProperty)
		// 			}
		// 		}
		// 	}
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// schemaRef400, err := openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil)
		// 	// if err != nil {
		// 	// 	zap.S().Error(err)
		// 	// 	schemaRef400 = new(openapi3.SchemaRef)
		// 	// }
		// 	// schemaRef400.Value.Example = exampleValue(response.CodeBadRequest)
		// 	// schemaRef404, err := openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil)
		// 	// if err != nil {
		// 	// 	zap.S().Error(err)
		// 	// 	schemaRef204 = new(openapi3.SchemaRef)
		// 	// }
		// 	// schemaRef404.Value.Example = exampleValue(response.CodeNotFound)
		// 	resp := openapi3.NewResponses()
		// 	resp.Set("204", &openapi3.ResponseRef{
		// 		Value: &openapi3.Response{
		// 			Description: util.ValueOf(fmt.Sprintf("%s deleted successfully", name)),
		// 			Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef204),
		// 		},
		// 	})
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// resp.Set("400", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf("Invalid request"),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef400),
		// 	// 	},
		// 	// })
		// 	// resp.Set("404", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef404),
		// 	// 	},
		// 	// })
		//
		// 	return resp
		// }(),
	}
	addHeaderParameters(pathItem.Delete)
}

func setUpdate[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	name := typ.Elem().Name()
	reqKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_UPDATE)
	rspKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_UPDATE)
	reqSchemaRef, _ := openapi3gen.NewSchemaRefForValue(*new(REQ), nil)
	rspSchemaRef, _ := openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil)
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Put = &openapi3.Operation{
		OperationID: operationID(consts.Update, typ),
		Summary:     summary(path, consts.Update, typ),
		Description: description(consts.Update, typ),
		Tags:        tags(path, consts.Update, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](200, rspKey),
		// RequestBody: &openapi3.RequestBodyRef{
		// 	Value: &openapi3.RequestBody{
		// 		Description: fmt.Sprintf("The %s data to update", name),
		// 		Required:    !model.IsModelEmpty[REQ](),
		// 		Content:     openapi3.NewContentWithJSONSchemaRef(reqSchemaRef),
		// 	},
		// },
		// Responses: func() *openapi3.Responses {
		// 	var schemaRef200 *openapi3.SchemaRef
		// 	// var schemaRef400 *openapi3.SchemaRef
		// 	// var schemaRef404 *openapi3.SchemaRef
		// 	var err error
		//
		// 	if schemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil); err == nil {
		// 		// Add field descriptions to response data schema
		// 		if schemaRef200.Value != nil && schemaRef200.Value.Properties != nil {
		// 			if dataProperty, exists := schemaRef200.Value.Properties["data"]; exists {
		// 				addSchemaTitleDesc[RSP](dataProperty)
		// 			}
		// 		}
		// 	}
		//
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// if schemaRef400, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil); err != nil {
		// 	// 	zap.S().Error(err)
		// 	// 	schemaRef400 = new(openapi3.SchemaRef)
		// 	// }
		// 	// schemaRef400.Value.Example = exampleValue(response.CodeBadRequest)
		// 	// if schemaRef404, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil); err != nil {
		// 	// 	zap.S().Error(err)
		// 	// 	schemaRef404 = new(openapi3.SchemaRef)
		// 	// }
		// 	// schemaRef404.Value.Example = exampleValue(response.CodeNotFound)
		//
		// 	resp := openapi3.NewResponses()
		// 	resp.Set("200", &openapi3.ResponseRef{
		// 		Value: &openapi3.Response{
		// 			Description: util.ValueOf(fmt.Sprintf("%s updated successfully", name)),
		// 			Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef200),
		// 			// Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{
		// 			// 	Ref: "#/components/schemas/" + typ.Elem().Name(),
		// 			// }),
		// 		},
		// 	})
		//
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// resp.Set("400", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf("Invalid request"),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef400),
		// 	// 	},
		// 	// })
		// 	// resp.Set("404", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef404),
		// 	// 	},
		// 	// })
		// 	return resp
		// }(),
	}
	addHeaderParameters(pathItem.Put)
	removeFieldsFromRequestBody(pathItem.Put)
}

func setPatch[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	name := typ.Elem().Name()
	reqKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_PATCH)
	rspKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_PATCH)
	reqSchemaRef, _ := openapi3gen.NewSchemaRefForValue(*new(REQ), nil)
	rspSchemaRef, _ := openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil)
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Patch = &openapi3.Operation{
		OperationID: operationID(consts.Patch, typ),
		Summary:     summary(path, consts.Patch, typ),
		Description: description(consts.Patch, typ),
		Tags:        tags(path, consts.Patch, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](200, rspKey),
		// RequestBody: &openapi3.RequestBodyRef{
		// 	Value: &openapi3.RequestBody{
		// 		Description: fmt.Sprintf("Partial fields of %s to update", name),
		// 		Required:    !model.IsModelEmpty[REQ](),
		// 		Content:     openapi3.NewContentWithJSONSchemaRef(reqSchemaRef),
		// 	},
		// },
		// Responses: func() *openapi3.Responses {
		// 	var schemaRef200 *openapi3.SchemaRef
		// 	// var schemaRef400 *openapi3.SchemaRef
		// 	// var schemaRef404 *openapi3.SchemaRef
		//
		// 	if schemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil); err == nil {
		// 		// Add field descriptions to response data schema
		// 		if schemaRef200.Value != nil && schemaRef200.Value.Properties != nil {
		// 			if dataProperty, exists := schemaRef200.Value.Properties["data"]; exists {
		// 				addSchemaTitleDesc[RSP](dataProperty)
		// 			}
		// 		}
		// 	}
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// if schemaRef400, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil); err != nil {
		// 	// 	zap.S().Error(err)
		// 	// 	schemaRef400 = new(openapi3.SchemaRef)
		// 	// }
		// 	// schemaRef400.Value.Example = exampleValue(response.CodeBadRequest)
		// 	// if schemaRef404, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil); err != nil {
		// 	// 	zap.S().Error(err)
		// 	// 	schemaRef404 = new(openapi3.SchemaRef)
		// 	// }
		// 	// schemaRef404.Value.Example = exampleValue(response.CodeNotFound)
		// 	resp := openapi3.NewResponses()
		// 	resp.Set("200", &openapi3.ResponseRef{
		// 		Value: &openapi3.Response{
		// 			Description: util.ValueOf(fmt.Sprintf("%s partially updated successfully", name)),
		// 			Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef200),
		// 			// Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{
		// 			// 	Ref: "#/components/schemas/" + typ.Elem().Name(),
		// 			// }),
		// 		},
		// 	})
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// resp.Set("400", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf("Invalid request"),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef400),
		// 	// 	},
		// 	// })
		// 	// resp.Set("404", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef404),
		// 	// 	},
		// 	// })
		// 	return resp
		// }(),
	}
	addHeaderParameters(pathItem.Patch)
	removeFieldsFromRequestBody(pathItem.Patch)
}

func setList[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	name := typ.Elem().Name()
	reqKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_LIST)
	rspKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_LIST)

	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		rspSchemaRef, _ = openapi3gen.NewSchemaRefForValue(*new(apiListResponse[M]), nil)
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		if dataProperty.Value != nil && dataProperty.Value.Properties != nil {
		// 			if itemsProperty, exists := dataProperty.Value.Properties["items"]; exists {
		// 				if itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 					addSchemaTitle[M](itemsProperty.Value.Items)
		// 				}
		// 			}
		// 		}
		// 	}
		// }
	} else {
		rspSchemaRef, _ = openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil)
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		addSchemaTitle[RSP](dataProperty)
		// 	}
		// }
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	pathItem.Get = &openapi3.Operation{
		OperationID: operationID(consts.List, typ),
		Summary:     summary(path, consts.List, typ),
		Description: description(consts.List, typ),
		Tags:        tags(path, consts.List, typ),
		Parameters:  parseParametersFromPath(path),
		Responses:   newResponses[RSP](200, rspKey),
		// // Parameters: []*openapi3.ParameterRef{
		// // 	{
		// // 		Value: &openapi3.Parameter{
		// // 			Name:     "page",
		// // 			In:       "query",
		// // 			Required: false,
		// // 			Schema: &openapi3.SchemaRef{
		// // 				Value: &openapi3.Schema{
		// // 					Type:    &openapi3.Types{openapi3.TypeInteger},
		// // 					Default: 1,
		// // 				},
		// // 			},
		// // 			Description: "Page number",
		// // 		},
		// // 	},
		// // 	{
		// // 		Value: &openapi3.Parameter{
		// // 			Name:     "pageSize",
		// // 			In:       "query",
		// // 			Required: false,
		// // 			Schema: &openapi3.SchemaRef{
		// // 				Value: &openapi3.Schema{
		// // 					Type:    &openapi3.Types{openapi3.TypeInteger},
		// // 					Default: 10,
		// // 				},
		// // 			},
		// // 			Description: "Number of items per page",
		// // 		},
		// // 	},
		// // 	// Can extend more query parameters, such as filter fields, sorting, etc.
		// // },
		// Responses: func() *openapi3.Responses {
		// 	var schemaRef200 *openapi3.SchemaRef
		// 	var err error
		// 	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		// 		if schemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiListResponse[M]), nil); err == nil {
		// 			// Add field descriptions to response data schema
		// 			if schemaRef200.Value != nil && schemaRef200.Value.Properties != nil {
		// 				if dataProperty, exists := schemaRef200.Value.Properties["data"]; exists {
		// 					if dataProperty.Value != nil && dataProperty.Value.Properties != nil {
		// 						if itemsProperty, exists := dataProperty.Value.Properties["items"]; exists {
		// 							if itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 								addSchemaTitleDesc[M](itemsProperty.Value.Items)
		// 							}
		// 						}
		// 					}
		// 				}
		// 			}
		// 		}
		// 	} else {
		// 		if !model.IsModelEmpty[RSP]() {
		// 			if schemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil); err == nil {
		// 				if schemaRef200.Value != nil && schemaRef200.Value.Properties != nil {
		// 					if dataProperty, exists := schemaRef200.Value.Properties["data"]; exists {
		// 						addSchemaTitleDesc[RSP](dataProperty)
		// 					}
		// 				}
		// 			}
		// 		}
		// 	}
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// schemaRef400, err := openapi3gen.NewSchemaRefForValue(*new(apiListResponse[string]), nil)
		// 	// if err != nil {
		// 	// 	zap.S().Error(err)
		// 	// 	schemaRef400 = new(openapi3.SchemaRef)
		// 	// }
		// 	// schemaRef400.Value.Example = exampleValue(response.CodeBadRequest)
		// 	// schemaRef404, err := openapi3gen.NewSchemaRefForValue(*new(apiListResponse[string]), nil)
		// 	// if err != nil {
		// 	// 	zap.S().Error(err)
		// 	// 	schemaRef404 = new(openapi3.SchemaRef)
		// 	// }
		// 	// schemaRef404.Value.Example = exampleValue(response.CodeNotFound)
		//
		// 	resp := openapi3.NewResponses()
		// 	resp.Set("200", &openapi3.ResponseRef{
		// 		Value: &openapi3.Response{
		// 			Description: util.ValueOf(fmt.Sprintf("List of %s", name)),
		// 			Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef200),
		// 		},
		// 	})
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// resp.Set("400", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(msgBadRequest),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef400),
		// 	// 	},
		// 	// })
		// 	// resp.Set("404", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(msgNotFound),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef404),
		// 	// 	},
		// 	// })
		//
		// 	return resp
		// }(),
	}
	addQueryParameters[M, REQ, RSP](pathItem.Get)
	addHeaderParameters(pathItem.Get)
}

func setGet[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	name := typ.Elem().Name()
	reqKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_GET)
	rspKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_GET)
	rspSchemaRef, _ := openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil)
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	pathItem.Get = &openapi3.Operation{
		OperationID: operationID(consts.Get, typ),
		Summary:     summary(path, consts.Get, typ),
		Description: description(consts.Get, typ),
		Tags:        tags(path, consts.Get, typ),
		Parameters:  parseParametersFromPath(path),
		Responses:   newResponses[RSP](200, rspKey),
		// Responses: func() *openapi3.Responses {
		// 	var schemaRef200 *openapi3.SchemaRef
		// 	var err error
		// 	if schemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil); err == nil {
		// 		// Add field descriptions to response data schema
		// 		if schemaRef200.Value != nil && schemaRef200.Value.Properties != nil {
		// 			if dataProperty, exists := schemaRef200.Value.Properties["data"]; exists {
		// 				addSchemaTitleDesc[RSP](dataProperty)
		// 			}
		// 		}
		// 	}
		//
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// schemaRef400, err := openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil)
		// 	// if err != nil {
		// 	// 	zap.S().Error(err)
		// 	// 	schemaRef400 = new(openapi3.SchemaRef)
		// 	// }
		// 	// schemaRef400.Value.Example = exampleValue(response.CodeBadRequest)
		// 	// schemaRef404, err := openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil)
		// 	// if err != nil {
		// 	// 	zap.S().Error(err)
		// 	// 	schemaRef404 = new(openapi3.SchemaRef)
		// 	// }
		// 	// schemaRef404.Value.Example = exampleValue(response.CodeNotFound)
		//
		// 	resp := openapi3.NewResponses()
		// 	resp.Set("200", &openapi3.ResponseRef{
		// 		Value: &openapi3.Response{
		// 			Description: util.ValueOf(fmt.Sprintf("%s detail", name)),
		// 			Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef200),
		// 			// Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{
		// 			// 	Ref: "#/components/schemas/" + name,
		// 			// }),
		// 		},
		// 	})
		//
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// resp.Set("400", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef400),
		// 	// 	},
		// 	// })
		// 	// resp.Set("404", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef404),
		// 	// 	},
		// 	// })
		// 	return resp
		// }(),
	}
	addHeaderParameters(pathItem.Get)
}

func setCreateMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	name := typ.Elem().Name()
	reqKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_CREATE_MANY)
	rspKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_CREATE_MANY)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		reqSchemaRef, _ = openapi3gen.NewSchemaRefForValue(*new(apiBatchRequest[REQ]), nil)
		// if reqSchemaRef.Value != nil && reqSchemaRef.Value.Properties != nil {
		// 	if itemsProperty, exists := reqSchemaRef.Value.Properties["items"]; exists && itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 		addSchemaTitle[M](itemsProperty.Value.Items)
		// 	}
		// }
		rspSchemaRef, _ = openapi3gen.NewSchemaRefForValue(*new(apiBatchResponse[RSP]), nil)
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		if dataProperty.Value != nil && dataProperty.Value.Properties != nil {
		// 			if itemsProperty, exists := dataProperty.Value.Properties["items"]; exists {
		// 				if itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 					addSchemaTitle[RSP](itemsProperty.Value.Items)
		// 				}
		// 			}
		// 		}
		// 	}
		// }
	} else {
		reqSchemaRef, _ = openapi3gen.NewSchemaRefForValue(*new(REQ), nil)
		rspSchemaRef, _ = openapi3gen.NewSchemaRefForValue(*new(RSP), nil)
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		addSchemaTitle[RSP](dataProperty)
		// 	}
		// }
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	// // // 定义 BatchCreateRequest schema
	// // reqSchemaName := name + "BatchRequest"
	// // reqSchemaRef := &openapi3.SchemaRef{
	// // 	Value: &openapi3.Schema{
	// // 		Type:     &openapi3.Types{openapi3.TypeObject},
	// // 		Required: []string{"items"},
	// // 		Properties: map[string]*openapi3.SchemaRef{
	// // 			"items": {
	// // 				Value: &openapi3.Schema{
	// // 					Type:  &openapi3.Types{openapi3.TypeArray},
	// // 					Items: &openapi3.SchemaRef{Ref: "#/components/schemas/" + name},
	// // 				},
	// // 			},
	// // 		},
	// // 	},
	// // }
	// // doc.Components.Schemas[reqSchemaName] = reqSchemaRef
	//
	// var err error
	// var reqSchemaRef *openapi3.SchemaRef
	// if reqSchemaRef, err = gen.NewSchemaRefForValue(*new(apiBatchRequest[REQ]), nil); err == nil {
	// 	// Add field descriptions to request body schema
	// 	if reqSchemaRef.Value != nil && reqSchemaRef.Value.Properties != nil {
	// 		if itemsProperty, exists := reqSchemaRef.Value.Properties["items"]; exists && itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
	// 			addSchemaTitleDesc[M](itemsProperty.Value.Items)
	// 		}
	// 	}
	// 	setupBatchExample(reqSchemaRef)
	// }

	pathItem.Post = &openapi3.Operation{
		OperationID: operationID(consts.CreateMany, typ),
		Summary:     summary(path, consts.CreateMany, typ),
		Description: description(consts.CreateMany, typ),
		Tags:        tags(path, consts.CreateMany, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](201, rspKey),
		// RequestBody: &openapi3.RequestBodyRef{
		// 	Value: &openapi3.RequestBody{
		// 		Description: fmt.Sprintf("Request body for batch creating %s", name),
		// 		Required:    true,
		// 		Content:     openapi3.NewContentWithJSONSchemaRef(reqSchemaRef),
		// 		// Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{
		// 		// 	Ref: "#/components/schemas/" + reqSchemaName,
		// 		// }),
		// 	},
		// },
		// Responses: func() *openapi3.Responses {
		// 	var rspSchemaRef200 *openapi3.SchemaRef
		// 	// var schemaRef400 *openapi3.SchemaRef
		// 	// var schemaRef404 *openapi3.SchemaRef
		// 	var err error
		//
		// 	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		// 		if rspSchemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiBatchResponse[M]), nil); err == nil {
		// 			// Add field descriptions to response data schema
		// 			if rspSchemaRef200.Value != nil && rspSchemaRef200.Value.Properties != nil {
		// 				if dataProperty, exists := rspSchemaRef200.Value.Properties["data"]; exists {
		// 					if dataProperty.Value != nil && dataProperty.Value.Properties != nil {
		// 						if itemsProperty, exists := dataProperty.Value.Properties["items"]; exists {
		// 							if itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 								addSchemaTitleDesc[M](itemsProperty.Value.Items)
		// 							}
		// 						}
		// 					}
		// 				}
		// 			}
		// 		}
		// 		// // Mybe used in the future, DO NOT DELETE it.
		// 		// if schemaRef400, err = openapi3gen.NewSchemaRefForValue(*new(apiBatchResponse[string]), nil); err != nil {
		// 		// 	zap.S().Error(err)
		// 		// 	schemaRef400 = new(openapi3.SchemaRef)
		// 		// }
		// 		// schemaRef400.Value.Example = exampleValue(response.CodeBadRequest)
		// 		// if schemaRef404, err = openapi3gen.NewSchemaRefForValue(*new(apiBatchResponse[string]), nil); err != nil {
		// 		// 	zap.S().Error(err)
		// 		// 	schemaRef404 = new(openapi3.SchemaRef)
		// 		// }
		// 		// schemaRef404.Value.Example = exampleValue(response.CodeNotFound)
		// 	} else {
		// 		if rspSchemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil); err == nil {
		// 			if rspSchemaRef200.Value != nil && rspSchemaRef200.Value.Properties != nil {
		// 				if dataProperty, exists := rspSchemaRef200.Value.Properties["data"]; exists {
		// 					addSchemaTitleDesc[RSP](dataProperty)
		// 				}
		// 			}
		// 		}
		// 	}
		//
		// 	resp := openapi3.NewResponses()
		// 	resp.Set("201", &openapi3.ResponseRef{
		// 		Value: &openapi3.Response{
		// 			Description: util.ValueOf(fmt.Sprintf("%s created", name)),
		// 			Content:     openapi3.NewContentWithJSONSchemaRef(rspSchemaRef200),
		// 		},
		// 	})
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// resp.Set("400", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef400),
		// 	// 	},
		// 	// })
		// 	// resp.Set("404", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef404),
		// 	// 	},
		// 	// })
		//
		// 	return resp
		// }(),
	}
	addHeaderParameters(pathItem.Post)
	removeFieldsFromBatchRequestBody(pathItem.Post)
}

func setDeleteMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	name := typ.Elem().Name()
	reqKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.DeleteMany)
	rspKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.DeleteMany)
	reqSchemaRef := &openapi3.SchemaRef{
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
	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		rspSchemaRef, _ = openapi3gen.NewSchemaRefForValue(*new(apiBatchResponse[RSP]), nil)
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists && dataProperty.Value != nil && dataProperty.Value.Properties != nil {
		// 		if itemsProperty, exists := dataProperty.Value.Properties["items"]; exists && itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 			addSchemaTitle[RSP](itemsProperty.Value.Items)
		// 		}
		// 	}
		// }
	} else {
		rspSchemaRef, _ = openapi3gen.NewSchemaRefForValue(*new(RSP), nil)
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		addSchemaTitle[RSP](dataProperty)
		// 	}
		// }
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Delete = &openapi3.Operation{
		OperationID: operationID(consts.DeleteMany, typ),
		Summary:     summary(path, consts.DeleteMany, typ),
		Description: description(consts.DeleteMany, typ),
		Tags:        tags(path, consts.DeleteMany, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](204, rspKey),
		// RequestBody: &openapi3.RequestBodyRef{
		// 	Value: &openapi3.RequestBody{
		// 		Required:    true,
		// 		Description: fmt.Sprintf("IDs of %s to delete", name),
		// 		Content:     openapi3.NewContentWithJSONSchemaRef(reqSchemaRef),
		// 	},
		// },
		// Responses: func() *openapi3.Responses {
		// 	var schemaRef200 *openapi3.SchemaRef
		// 	var err error
		//
		// 	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		// 		if schemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiBatchResponse[M]), nil); err == nil {
		// 			// Add field descriptions to response data schema
		// 			if schemaRef200.Value != nil && schemaRef200.Value.Properties != nil {
		// 				if dataProperty, exists := schemaRef200.Value.Properties["data"]; exists && dataProperty.Value != nil && dataProperty.Value.Properties != nil {
		// 					if itemsProperty, exists := dataProperty.Value.Properties["items"]; exists && itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 						addSchemaTitleDesc[M](itemsProperty.Value.Items)
		// 					}
		// 				}
		// 			}
		// 		}
		// 		// // Mybe used in the future, DO NOT DELETE it.
		// 		// schemaRef400, err := openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil)
		// 		// schemaRef404, err := openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil)
		// 	} else {
		// 		if schemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil); err == nil {
		// 			if schemaRef200.Value != nil && schemaRef200.Value.Properties != nil {
		// 				if dataProperty, exists := schemaRef200.Value.Properties["data"]; exists {
		// 					addSchemaTitleDesc[RSP](dataProperty)
		// 				}
		// 			}
		// 		}
		// 	}
		//
		// 	resp := openapi3.NewResponses()
		// 	resp.Set("200", &openapi3.ResponseRef{
		// 		Value: &openapi3.Response{
		// 			Description: util.ValueOf(fmt.Sprintf("%s deleted", name)),
		// 			Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef200),
		// 		},
		// 	})
		//
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// resp.Set("400", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef400),
		// 	// 	},
		// 	// })
		// 	// resp.Set("404", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef404),
		// 	// 	},
		// 	// })
		// 	return resp
		// }(),
	}
	addHeaderParameters(pathItem.Delete)
}

func setUpdateMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	gen := openapi3gen.NewGenerator()
	typ := reflect.TypeOf(*new(M))
	name := typ.Elem().Name()
	reqKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_UPDATE_MANY)
	rspKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_UPDATE_MANY)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		reqSchemaRef, _ = gen.NewSchemaRefForValue(*new(apiBatchRequest[REQ]), nil)
		// if reqSchemaRef.Value != nil && reqSchemaRef.Value.Properties != nil {
		// 	if itemsProperty, exists := reqSchemaRef.Value.Properties["items"]; exists && itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 		addSchemaTitle[M](itemsProperty.Value.Items)
		// 	}
		// }
		rspSchemaRef, _ = openapi3gen.NewSchemaRefForValue(*new(apiBatchResponse[REQ]), nil)
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		if dataProperty.Value != nil && dataProperty.Value.Properties != nil {
		// 			if itemsProperty, exists := dataProperty.Value.Properties["items"]; exists {
		// 				if itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 					addSchemaTitle[REQ](itemsProperty.Value.Items)
		// 				}
		// 			}
		// 		}
		// 	}
		// }
	} else {
		reqSchemaRef, _ = gen.NewSchemaRefForValue(*new(REQ), nil)
		rspSchemaRef, _ = openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil)
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		addSchemaTitle[RSP](dataProperty)
		// 	}
		// }
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Put = &openapi3.Operation{
		OperationID: operationID(consts.UpdateMany, typ),
		Summary:     summary(path, consts.UpdateMany, typ),
		Description: description(consts.UpdateMany, typ),
		Tags:        tags(path, consts.UpdateMany, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](200, rspKey),
		// RequestBody: &openapi3.RequestBodyRef{
		// 	Value: &openapi3.RequestBody{
		// 		Description: fmt.Sprintf("Request body for batch updating %s", name),
		// 		Required:    true,
		// 		Content:     openapi3.NewContentWithJSONSchemaRef(reqSchemaRef),
		// 	},
		// },
		// Responses: func() *openapi3.Responses {
		// 	var rspSchemaRef200 *openapi3.SchemaRef
		// 	// var schemaRef400 *openapi3.SchemaRef
		// 	// var schemaRef404 *openapi3.SchemaRef
		//
		// 	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		// 		if rspSchemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiBatchResponse[RSP]), nil); err == nil {
		// 			// Add field descriptions to response data schema
		// 			if rspSchemaRef200.Value != nil && rspSchemaRef200.Value.Properties != nil {
		// 				if dataProperty, exists := rspSchemaRef200.Value.Properties["data"]; exists {
		// 					if dataProperty.Value != nil && dataProperty.Value.Properties != nil {
		// 						if itemsProperty, exists := dataProperty.Value.Properties["items"]; exists {
		// 							if itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 								addSchemaTitleDesc[M](itemsProperty.Value.Items)
		// 							}
		// 						}
		// 					}
		// 				}
		// 			}
		// 		}
		// 		// // Mybe used in the future, DO NOT DELETE it.
		// 		// if schemaRef400, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil); err != nil {
		// 		// 	zap.S().Error(err)
		// 		// 	schemaRef400 = new(openapi3.SchemaRef)
		// 		// }
		// 		// schemaRef400.Value.Example = exampleValue(response.CodeBadRequest)
		// 		// if schemaRef404, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil); err != nil {
		// 		// 	zap.S().Error(err)
		// 		// 	schemaRef404 = new(openapi3.SchemaRef)
		// 		// }
		// 		// schemaRef404.Value.Example = exampleValue(response.CodeNotFound)
		// 	} else {
		// 		if rspSchemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[RSP]), nil); err == nil {
		// 			// Add field descriptions to response data schema
		// 			if rspSchemaRef200.Value != nil && rspSchemaRef200.Value.Properties != nil {
		// 				if dataProperty, exists := rspSchemaRef200.Value.Properties["data"]; exists {
		// 					addSchemaTitleDesc[RSP](dataProperty)
		// 				}
		// 			}
		// 		}
		// 	}
		// 	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef200)
		//
		// 	resp := openapi3.NewResponses()
		// 	resp.Set("200", &openapi3.ResponseRef{
		// 		Value: &openapi3.Response{
		// 			Description: util.ValueOf(fmt.Sprintf("%s updated", name)),
		// 			Content:     openapi3.NewContentWithJSONSchemaRef(rspSchemaRef200),
		// 		},
		// 	})
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// resp.Set("400", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef400),
		// 	// 	},
		// 	// })
		// 	// resp.Set("404", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef404),
		// 	// 	},
		// 	// })
		//
		// 	return resp
		// }(),
	}
	addHeaderParameters(pathItem.Put)
	removeFieldsFromBatchRequestBody(pathItem.Put)
}

func setPatchMany[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	gen := openapi3gen.NewGenerator()
	typ := reflect.TypeOf(*new(M))
	name := typ.Elem().Name()
	reqKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_PATCH_MANY)
	rspKey := fmt.Sprintf("%s_%s", strings.ToLower(name), consts.PHASE_PATCH_MANY)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		reqSchemaRef, _ = gen.NewSchemaRefForValue(*new(apiBatchRequest[REQ]), nil)
		// if reqSchemaRef.Value != nil && reqSchemaRef.Value.Properties != nil {
		// 	if itemsProperty, exists := reqSchemaRef.Value.Properties["items"]; exists && itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 		addSchemaTitle[M](itemsProperty.Value.Items)
		// 	}
		// }
		rspSchemaRef, _ = gen.NewSchemaRefForValue(*new(apiBatchResponse[RSP]), nil)
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		if dataProperty.Value != nil && dataProperty.Value.Properties != nil {
		// 			if itemsProperty, exists := dataProperty.Value.Properties["items"]; exists {
		// 				if itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 					addSchemaTitle[M](itemsProperty.Value.Items)
		// 				}
		// 			}
		// 		}
		// 	}
		// }
	} else {
		reqSchemaRef, _ = gen.NewSchemaRefForValue(*new(REQ), nil)
		rspSchemaRef, _ = gen.NewSchemaRefForValue(*new(RSP), nil)
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		addSchemaTitle[RSP](dataProperty)
		// 	}
		// }
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Patch = &openapi3.Operation{
		OperationID: operationID(consts.PatchMany, typ),
		Summary:     summary(path, consts.PatchMany, typ),
		Description: description(consts.PatchMany, typ),
		Tags:        tags(path, consts.PatchMany, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](200, rspKey),
		// RequestBody: &openapi3.RequestBodyRef{
		// 	Value: &openapi3.RequestBody{
		// 		Description: fmt.Sprintf("Request body for batch partial updating %s", name),
		// 		Required:    true,
		// 		Content:     openapi3.NewContentWithJSONSchemaRef(reqSchemaRef),
		// 	},
		// },
		// Responses: func() *openapi3.Responses {
		// 	var rspSchemaRef200 *openapi3.SchemaRef
		// 	// var schemaRef400 *openapi3.SchemaRef
		// 	// var schemaRef404 *openapi3.SchemaRef
		// 	var err error
		//
		// 	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		// 		if rspSchemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiBatchResponse[RSP]), nil); err == nil {
		// 			// Add field descriptions to response data schema
		// 			if rspSchemaRef200.Value != nil && rspSchemaRef200.Value.Properties != nil {
		// 				if dataProperty, exists := rspSchemaRef200.Value.Properties["data"]; exists {
		// 					if dataProperty.Value != nil && dataProperty.Value.Properties != nil {
		// 						if itemsProperty, exists := dataProperty.Value.Properties["items"]; exists {
		// 							if itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 								addSchemaTitleDesc[M](itemsProperty.Value.Items)
		// 							}
		// 						}
		// 					}
		// 				}
		// 			}
		// 		}
		// 		// // Mybe used in the future, DO NOT DELETE it.
		// 		// if schemaRef400, err = openapi3gen.NewSchemaRefForValue(*new(apiBatchResponse[string]), nil); err != nil {
		// 		// 	zap.S().Error(err)
		// 		// 	schemaRef400 = new(openapi3.SchemaRef)
		// 		// }
		// 		// schemaRef400.Value.Example = exampleValue(response.CodeBadRequest)
		// 		// if schemaRef404, err = openapi3gen.NewSchemaRefForValue(*new(apiBatchResponse[string]), nil); err != nil {
		// 		// 	zap.S().Error(err)
		// 		// 	schemaRef404 = new(openapi3.SchemaRef)
		// 		// }
		// 		// schemaRef404.Value.Example = exampleValue(response.CodeNotFound)
		// 	} else {
		// 		if rspSchemaRef200, err = openapi3gen.NewSchemaRefForValue(*new(apiResponse[string]), nil); err == nil {
		// 			if rspSchemaRef200.Value != nil && rspSchemaRef200.Value.Properties != nil {
		// 				if dataProperty, exists := rspSchemaRef200.Value.Properties["data"]; exists {
		// 					addSchemaTitleDesc[RSP](dataProperty)
		// 				}
		// 			}
		// 		}
		// 	}
		//
		// 	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef200)
		// 	resp := openapi3.NewResponses()
		// 	resp.Set("200", &openapi3.ResponseRef{
		// 		Value: &openapi3.Response{
		// 			Description: util.ValueOf(fmt.Sprintf("%s partially updated", name)),
		// 			Content:     openapi3.NewContentWithJSONSchemaRef(rspSchemaRef200),
		// 		},
		// 	})
		// 	// // Mybe used in the future, DO NOT DELETE it.
		// 	// resp.Set("400", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef400),
		// 	// 	},
		// 	// })
		// 	// resp.Set("404", &openapi3.ResponseRef{
		// 	// 	Value: &openapi3.Response{
		// 	// 		Description: util.ValueOf(fmt.Sprintf("%s not found", name)),
		// 	// 		Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef404),
		// 	// 	},
		// 	// })
		//
		// 	return resp
		// }(),
	}
	addHeaderParameters(pathItem.Patch)
	removeFieldsFromBatchRequestBody(pathItem.Patch)
}

func setImport[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	// pathItem.Post = &openapi3.Operation{
	// 	OperationID: "import" + reflect.TypeOf(*new(M)).Elem().Name(),
	// 	Summary:     "Import " + reflect.TypeOf(*new(M)).Elem().Name() + " data",
	// 	Description: "Import data from CSV/Excel file",
	// 	Tags:        tags(path, "import", reflect.TypeOf(*new(M))),
	// 	RequestBody: &openapi3.RequestBodyRef{
	// 		Value: &openapi3.RequestBody{
	// 			Description: "File to import",
	// 			Required:    true,
	// 			Content: openapi3.Content{
	// 				"multipart/form-data": &openapi3.MediaType{
	// 					Schema: &openapi3.SchemaRef{
	// 						Value: &openapi3.Schema{
	// 							Type: &openapi3.Types{openapi3.TypeObject},
	// 							Properties: map[string]*openapi3.SchemaRef{
	// 								"file": {
	// 									Value: &openapi3.Schema{
	// 										Type:   &openapi3.Types{openapi3.TypeString},
	// 										Format: "binary",
	// 									},
	// 								},
	// 							},
	// 							Required: []string{"file"},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	},
	// 	Responses: newResponses(200, "ImportResponse"),
	// }
}

func setExport[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	// pathItem.Get = &openapi3.Operation{
	// 	OperationID: "export" + reflect.TypeOf(*new(M)).Elem().Name(),
	// 	Summary:     "Export " + reflect.TypeOf(*new(M)).Elem().Name() + " data",
	// 	Description: "Export data to CSV/Excel file",
	// 	Tags:        tags(path, "export", reflect.TypeOf(*new(M))),
	// 	Parameters: []*openapi3.ParameterRef{
	// 		{
	// 			Value: &openapi3.Parameter{
	// 				Name:        "format",
	// 				In:          "query",
	// 				Description: "Export format",
	// 				Schema: &openapi3.SchemaRef{
	// 					Value: &openapi3.Schema{
	// 						Type:    &openapi3.Types{openapi3.TypeString},
	// 						Enum:    []any{"csv", "xlsx"},
	// 						Default: "csv",
	// 					},
	// 				},
	// 			},
	// 		},
	// 	},
	// 	Responses: &openapi3.Responses{
	// 		MapOfResponseOrRefValues: openapi3.ResponsesMap{
	// 			"200": &openapi3.ResponseRef{
	// 				Value: &openapi3.Response{
	// 					Description: util.ValueOf("Export file"),
	// 					Content: openapi3.Content{
	// 						"text/csv": &openapi3.MediaType{
	// 							Schema: &openapi3.SchemaRef{
	// 								Value: &openapi3.Schema{
	// 									Type:   &openapi3.Types{openapi3.TypeString},
	// 									Format: "binary",
	// 								},
	// 							},
	// 						},
	// 						"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": &openapi3.MediaType{
	// 							Schema: &openapi3.SchemaRef{
	// 								Value: &openapi3.Schema{
	// 									Type:   &openapi3.Types{openapi3.TypeString},
	// 									Format: "binary",
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	},
	// }
}

// register Model, Model Payload, Model Result into openapi3 schema.
func registerSchema[M types.Model, REQ types.Request, RSP types.Response](reqKey, rspKey string, reqSchemaRef *openapi3.SchemaRef, rspSchemaRef *openapi3.SchemaRef) {
	if !modelregistry.IsEmpty[M]() {
		typ := reflect.TypeOf(*new(M))
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		name := typ.Name()
		docMutex.Lock()
		if doc.Components.Schemas == nil {
			doc.Components.Schemas = openapi3.Schemas{}
		}
		if _, ok := doc.Components.Schemas[name]; !ok {
			if schemaRef, err := openapi3gen.NewSchemaRefForValue(*new(M), nil); err == nil {
				addSchemaTitle[M](schemaRef)
				doc.Components.Schemas[name] = schemaRef
			}
		}
		docMutex.Unlock()
	}

	if !modelregistry.IsEmpty[REQ]() {
		typ := reflect.TypeOf(*new(M))
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		name := typ.Name()

		docMutex.Lock()
		if doc.Components.RequestBodies == nil {
			doc.Components.RequestBodies = openapi3.RequestBodies{}
		}
		if _, ok := doc.Components.RequestBodies[reqKey]; !ok && reqSchemaRef != nil {
			processAllRequestTypes[REQ](reqSchemaRef)
			setupExample(reqSchemaRef)
			setupBatchExample(reqSchemaRef)
			doc.Components.RequestBodies[reqKey] = &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Description: name + " Payload",
					Required:    !modelregistry.IsEmpty[REQ](),
					Content:     openapi3.NewContentWithJSONSchemaRef(reqSchemaRef),
				},
			}

		}
		docMutex.Unlock()
	}

	if !modelregistry.IsEmpty[RSP]() {
		typ := reflect.TypeOf(*new(M))
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		name := typ.Name()

		docMutex.Lock()
		if doc.Components.Responses == nil {
			doc.Components.Responses = openapi3.ResponseBodies{}
		}
		if _, ok := doc.Components.Responses[rspKey]; !ok && rspSchemaRef != nil {
			processAllResponseTypes[RSP](rspSchemaRef)
			doc.Components.Responses[rspKey] = &openapi3.ResponseRef{
				Value: &openapi3.Response{
					Description: new(name + " Response"),
					Content:     openapi3.NewContentWithJSONSchemaRef(rspSchemaRef),
				},
			}
			// if schemaRef, err := openapi3gen.NewSchemaRefForValue(*new(RSP), nil); err == nil {
			// 	addSchemaTitleDesc[RSP](schemaRef)
			// 	doc.Components.Responses[rspKey] = &openapi3.ResponseRef{
			// 		Value: &openapi3.Response{
			// 			Description: util.ValueOf(fmt.Sprintf("%s result", name)),
			// 			Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef),
			// 		},
			// 	}
			// }
		}
		docMutex.Unlock()
	}
}

// processAllRequestTypes 统一处理所有类型的 Request schema
func processAllRequestTypes[REQ types.Request](reqSchemaRef *openapi3.SchemaRef) {
	if reqSchemaRef == nil || reqSchemaRef.Value == nil {
		return
	}

	// 如果是普通请求，直接处理
	if len(reqSchemaRef.Value.Properties) == 0 {
		addSchemaTitle[REQ](reqSchemaRef)
		return
	}

	// 检查是否是批量请求（有 items 字段）
	if itemsProperty, hasItems := reqSchemaRef.Value.Properties["items"]; hasItems {
		if itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
			// 为批量请求的 items 添加注释
			addSchemaTitle[REQ](itemsProperty.Value.Items)
		}
	} else {
		// 普通请求
		addSchemaTitle[REQ](reqSchemaRef)
	}
}

// processAllResponseTypes 统一处理所有类型的 Response schema
func processAllResponseTypes[RSP types.Response](rspSchemaRef *openapi3.SchemaRef) {
	if rspSchemaRef == nil || rspSchemaRef.Value == nil || rspSchemaRef.Value.Properties == nil {
		return
	}

	// 处理 data 字段
	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists && dataProperty.Value != nil {
		// 检查 data 是什么类型的结构

		// 1. 如果 data 直接是 RSP 类型（普通的 apiResponse[RSP]）
		if len(dataProperty.Value.Properties) == 0 {
			// data 是一个简单类型或者没有嵌套属性
			addSchemaTitle[RSP](dataProperty)
		} else {
			// 2. 检查是否是 apiListResponse（有 items 和 total）
			if itemsProperty, hasItems := dataProperty.Value.Properties["items"]; hasItems {
				if totalProperty, hasTotal := dataProperty.Value.Properties["total"]; hasTotal && totalProperty != nil {
					// 这是 apiListResponse 类型
					if itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
						addSchemaTitle[RSP](itemsProperty.Value.Items)
					}
				} else if summaryProperty, hasSummary := dataProperty.Value.Properties["summary"]; hasSummary && summaryProperty != nil {
					// 3. 这是 apiBatchResponse 类型（有 items, options, summary）
					if itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
						addSchemaTitle[RSP](itemsProperty.Value.Items)
					}
				}
			} else {
				// 4. 可能是直接的 RSP 类型，但有嵌套属性
				addSchemaTitle[RSP](dataProperty)
			}
		}
	}
}

func parseParametersFromPath(path string) []*openapi3.ParameterRef {
	// re := regexp.MustCompile(`{(.+?)}`)
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)

	var params []string
	for _, m := range matches {
		if len(m) > 1 {
			params = append(params, m[1])
		}
	}

	parameterRefList := make([]*openapi3.ParameterRef, 0, len(params))

	for _, param := range params {
		parameterRefList = append(parameterRefList, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				In:       "path",
				Name:     param,
				Required: true,
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:   &openapi3.Types{openapi3.TypeString},
						Format: idFormat,
					},
				},
			},
		})
	}

	return parameterRefList
}

// setupExample will remove field "created_at", "created_by", "updated_at", "updated_by", "id".
//
// Before:
//
//	{
//	  "created_at": "2025-04-19T19:22:55.434Z",
//	  "created_by": "string",
//	  "desc": "string",
//	  "id": "string",
//	  "member_count": 0,
//	  "name": "string",
//	  "updated_at": "2025-04-19T19:22:55.434Z",
//	  "updated_by": "string"
//	}
//
// After:
//
//	{
//	  "desc": "string",
//	  "member_count": 0,
//	  "name": "string",
//	}
//
// NOTE: 结构体字段必须有 json tag, 否则 schemaRef.Value.Properties 中不会带有这些字段
func setupExample(schemaRef *openapi3.SchemaRef) {
	if schemaRef == nil {
		return
	}
	if schemaRef.Value == nil {
		schemaRef.Value = new(openapi3.Schema)
	}

	examples := make(map[string]any)
	for k, v := range schemaRef.Value.Properties {
		if removeFieldMap[k] {
			continue
		}
		if v.Value == nil {
			continue
		}
		examples[k] = buildExampleValue(v.Value, 0)
	}
	schemaRef.Value.Example = examples
}

// maxExampleDepth bounds buildExampleValue recursion so a self-referential
// type (eg. a tree or linked-list struct) can't recurse indefinitely.
const maxExampleDepth = 10

// buildExampleValue recursively builds an example value for schema so nested
// arrays, structs, and maps (additionalProperties) show their full shape in
// Swagger instead of an empty placeholder.
func buildExampleValue(schema *openapi3.Schema, depth int) any {
	if schema == nil || schema.Type == nil || depth > maxExampleDepth {
		return nil
	}

	switch {
	case schema.Type.Is(openapi3.TypeString):
		return "string"
	case schema.Type.Is(openapi3.TypeInteger):
		return 0
	case schema.Type.Is(openapi3.TypeNumber):
		return 0.0
	case schema.Type.Is(openapi3.TypeBoolean):
		return false
	case schema.Type.Is(openapi3.TypeArray):
		if schema.Items == nil || schema.Items.Value == nil {
			return []any{}
		}
		return []any{buildExampleValue(schema.Items.Value, depth+1)}
	case schema.Type.Is(openapi3.TypeObject):
		if len(schema.Properties) > 0 {
			example := make(map[string]any, len(schema.Properties))
			for propName, propRef := range schema.Properties {
				if removeFieldMap[propName] || propRef.Value == nil {
					continue
				}
				example[propName] = buildExampleValue(propRef.Value, depth+1)
			}
			return example
		}
		if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
			return map[string]any{"string": buildExampleValue(schema.AdditionalProperties.Schema.Value, depth+1)}
		}
		return map[string]any{}
	default:
		return nil
	}
}

func setupBatchExample(schemaRef *openapi3.SchemaRef) {
	if schemaRef == nil || schemaRef.Value == nil {
		return
	}

	props := schemaRef.Value.Properties
	for k, v := range props {
		if k == "items" && v.Value != nil && v.Value.Type.Is(openapi3.TypeArray) {
			if v.Value.Items != nil && v.Value.Items.Value != nil {
				// 为数组中的单个元素创建 example
				example := make(map[string]any)
				for propName, propRef := range v.Value.Items.Value.Properties {
					if removeFieldMap[propName] || propRef.Value == nil {
						continue
					}
					example[propName] = buildExampleValue(propRef.Value, 0)
				}

				// 设置单个 item 的 example
				v.Value.Items.Value.Example = example

				// 设置整个 batch request 的 example
				schemaRef.Value.Example = map[string]any{
					"items": []map[string]any{example},
				}
			}
		}
	}
}

// removeFieldsFromRequestBody 从单个 CRUD 操作的 RequestBody 中移除指定字段
func removeFieldsFromRequestBody(op *openapi3.Operation, fieldsToRemove ...string) {
	if op == nil || op.RequestBody == nil {
		return
	}

	// 创建一个 map 方便查找
	removeMap := make(map[string]bool)
	for _, field := range fieldsToRemove {
		removeMap[field] = true
	}

	// 如果默认没有传入要移除的字段，使用默认值
	if len(fieldsToRemove) == 0 {
		removeMap = removeFieldMap
	}

	// 处理 RequestBodyRef
	var requestBody *openapi3.RequestBody

	if op.RequestBody.Ref != "" {
		// 如果是引用，需要从 components 中获取实际的 RequestBody
		docMutex.RLock()
		if doc.Components.RequestBodies != nil {
			refKey := strings.TrimPrefix(op.RequestBody.Ref, "#/components/requestBodies/")
			if rb, exists := doc.Components.RequestBodies[refKey]; exists && rb.Value != nil {
				requestBody = rb.Value
			}
		}
		docMutex.RUnlock()
	} else if op.RequestBody.Value != nil {
		requestBody = op.RequestBody.Value
	}

	if requestBody == nil || requestBody.Content == nil {
		return
	}

	// 使用写锁保护对 schema 的修改操作
	docMutex.Lock()
	defer docMutex.Unlock()

	// 处理每个 content type
	for contentType, mediaType := range requestBody.Content {
		if mediaType.Schema == nil || mediaType.Schema.Value == nil {
			continue
		}

		schema := mediaType.Schema.Value

		// 移除 properties 中的字段
		if schema.Properties != nil {
			for field := range removeMap {
				delete(schema.Properties, field)
			}
		}

		// 移除 required 中的字段
		if len(schema.Required) > 0 {
			newRequired := []string{}
			for _, req := range schema.Required {
				if !removeMap[req] {
					newRequired = append(newRequired, req)
				}
			}
			schema.Required = newRequired
		}

		// 处理 example
		if schema.Example != nil {
			if exampleMap, ok := schema.Example.(map[string]any); ok {
				for field := range removeMap {
					delete(exampleMap, field)
				}
			}
		}

		// 更新 content
		requestBody.Content[contentType] = mediaType
	}
}

// removeFieldsFromBatchRequestBody 从批量 CRUD 操作的 RequestBody 中移除指定字段
func removeFieldsFromBatchRequestBody(op *openapi3.Operation, fieldsToRemove ...string) {
	if op == nil || op.RequestBody == nil {
		return
	}

	// 创建一个 map 方便查找
	removeMap := make(map[string]bool)
	for _, field := range fieldsToRemove {
		removeMap[field] = true
	}

	// 如果默认没有传入要移除的字段，使用默认值
	if len(fieldsToRemove) == 0 {
		removeMap = removeFieldMap
	}

	// 处理 RequestBodyRef
	var requestBody *openapi3.RequestBody

	if op.RequestBody.Ref != "" {
		// 如果是引用，需要从 components 中获取实际的 RequestBody
		docMutex.RLock()
		if doc.Components.RequestBodies != nil {
			refKey := strings.TrimPrefix(op.RequestBody.Ref, "#/components/requestBodies/")
			if rb, exists := doc.Components.RequestBodies[refKey]; exists && rb.Value != nil {
				requestBody = rb.Value
			}
		}
		docMutex.RUnlock()
	} else if op.RequestBody.Value != nil {
		requestBody = op.RequestBody.Value
	}

	if requestBody == nil || requestBody.Content == nil {
		return
	}

	// 使用写锁保护对 schema 的修改操作
	docMutex.Lock()
	defer docMutex.Unlock()

	// 处理每个 content type
	for contentType, mediaType := range requestBody.Content {
		if mediaType.Schema == nil || mediaType.Schema.Value == nil {
			continue
		}

		schema := mediaType.Schema.Value

		// 对于批量操作，需要处理 items 数组
		if schema.Properties != nil {
			if itemsProp, exists := schema.Properties["items"]; exists {
				if itemsProp.Value != nil && itemsProp.Value.Items != nil && itemsProp.Value.Items.Value != nil {
					itemSchema := itemsProp.Value.Items.Value

					// 移除 items 中每个元素的字段
					if itemSchema.Properties != nil {
						for field := range removeMap {
							delete(itemSchema.Properties, field)
						}
					}

					// 移除 required 中的字段
					if len(itemSchema.Required) > 0 {
						newRequired := []string{}
						for _, req := range itemSchema.Required {
							if !removeMap[req] {
								newRequired = append(newRequired, req)
							}
						}
						itemSchema.Required = newRequired
					}

					// 处理 items 的 example
					if itemSchema.Example != nil {
						if exampleMap, ok := itemSchema.Example.(map[string]any); ok {
							for field := range removeMap {
								delete(exampleMap, field)
							}
						}
					}
				}
			}
		}

		// 处理整个 batch request 的 example
		if schema.Example != nil {
			if exampleMap, ok := schema.Example.(map[string]any); ok {
				if items, exists := exampleMap["items"]; exists {
					if itemsArray, ok := items.([]map[string]any); ok {
						for _, item := range itemsArray {
							for field := range removeMap {
								delete(item, field)
							}
						}
					} else if itemsArray, ok := items.([]any); ok {
						for i, item := range itemsArray {
							if itemMap, ok := item.(map[string]any); ok {
								for field := range removeMap {
									delete(itemMap, field)
								}
								itemsArray[i] = itemMap
							}
						}
					}
				}
			}
		}

		// 更新 content
		requestBody.Content[contentType] = mediaType
	}
}

// func setupBatchExample(schemaRef *openapi3.SchemaRef) {
// 	if schemaRef == nil {
// 		return
// 	}
// 	if schemaRef.Value == nil {
// 		schemaRef.Value = new(openapi3.Schema)
// 	}
// 	props := schemaRef.Value.Properties
// 	for k, v := range props {
// 		if k == "items" && v.Value.Type.Is(openapi3.TypeArray) {
// 			example := make(map[string]any)
// 			for k, v := range v.Value.Items.Value.Properties {
// 				if k == "created_at" || k == "created_by" || k == "updated_at" || k == "updated_by" {
// 					continue
// 				}
// 				if v.Value == nil || v.Value.Type == nil {
// 					continue
// 				}
// 				if v.Value.Type.Is(openapi3.TypeString) {
// 					example[k] = "string"
// 				}
// 				if v.Value.Type.Is(openapi3.TypeInteger) {
// 					example[k] = 0
// 				}
// 				if v.Value.Type.Is(openapi3.TypeNumber) {
// 					example[k] = 0.0
// 				}
// 				if v.Value.Type.Is(openapi3.TypeBoolean) {
// 					example[k] = false
// 				}
// 				if v.Value.Type.Is(openapi3.TypeArray) {
// 					example[k] = []any{}
// 				}
// 				if v.Value.Type.Is(openapi3.TypeObject) {
// 					example[k] = map[string]any{}
// 				}
// 				if v.Value.Type.Is(openapi3.TypeNull) {
// 					example[k] = nil
// 				}
// 			}
// 			v.Value.Items.Value.Example = example
// 		}
// 	}
// }

func addHeaderParameters(op *openapi3.Operation) {
	headers := []*openapi3.ParameterRef{
		// // Mybe used in the future, DO NOT DELETE it.
		// {
		// 	Value: &openapi3.Parameter{
		// 		In:          "header",
		// 		Name:        "Authorization",
		// 		Description: "Authentication token (e.g. Bearer <token>)",
		// 		Required:    false,
		// 		Schema: &openapi3.SchemaRef{
		// 			Value: &openapi3.Schema{
		// 				Type: &openapi3.Types{openapi3.TypeString},
		// 			},
		// 		},
		// 	},
		// },
		// {
		// 	Value: &openapi3.Parameter{
		// 		In:          "header",
		// 		Name:        "X-Trace-ID",
		// 		Description: "Optional trace ID for tracing",
		// 		Required:    false,
		// 		Schema: &openapi3.SchemaRef{
		// 			Value: &openapi3.Schema{
		// 				Type: &openapi3.Types{openapi3.TypeString},
		// 			},
		// 		},
		// 	},
		// },
		// {
		// 	Value: &openapi3.Parameter{
		// 		In:          "header",
		// 		Name:        "X-Client-Version",
		// 		Description: "Client version (e.g. v1.2.3)",
		// 		Required:    false,
		// 		Schema: &openapi3.SchemaRef{
		// 			Value: &openapi3.Schema{
		// 				Type: &openapi3.Types{openapi3.TypeString},
		// 			},
		// 		},
		// 	},
		// },
		// {
		// 	Value: &openapi3.Parameter{
		// 		In:          "header",
		// 		Name:        "Accept-Language",
		// 		Description: "Preferred language (e.g. zh-CN, en-US)",
		// 		Required:    false,
		// 		Schema: &openapi3.SchemaRef{
		// 			Value: &openapi3.Schema{
		// 				Type: &openapi3.Types{openapi3.TypeString},
		// 			},
		// 		},
		// 	},
		// },
	}

	// Avoid duplicate additions
	existing := map[string]bool{}
	for _, p := range op.Parameters {
		if p.Value != nil {
			existing[p.Value.Name] = true
		}
	}

	for _, header := range headers {
		if header.Value != nil && !existing[header.Value.Name] {
			op.Parameters = append(op.Parameters, header)
		}
	}
}

var (
	// Cache field descriptions of model.Base to avoid frequent parsing
	baseModelDocsCache map[string]string
	baseModelDocsOnce  sync.Once
)

// getBaseModelDocs gets field descriptions of model.Base (with caching)
func getBaseModelDocs() map[string]string {
	baseModelDocsOnce.Do(func() {
		baseModel := &model.Base{}
		baseModelDocsCache = parseModelDocs(baseModel)
	})
	return baseModelDocsCache
}

// addSchemaTitle adds field titles to schema properties
func addSchemaTitle[T any](schemaRef *openapi3.SchemaRef) {
	if schemaRef == nil || schemaRef.Value == nil || schemaRef.Value.Properties == nil {
		return
	}

	// Get model field descriptions
	modelInstance := *new(T)
	modelDocs := parseModelDocs(modelInstance)

	// Get field descriptions of model.Base (using cache)
	baseDocs := getBaseModelDocs()

	// Create a mapping from JSON property names to field descriptions
	propertyDescriptions := make(map[string]string)
	fieldByJSON := make(map[string]reflect.StructField)

	// Process model fields
	typ := reflect.TypeOf(*new(T))
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	for field := range typ.Fields() {
		jsonTag := getFieldTag(field, consts.TAG_JSON)
		if len(jsonTag) == 0 {
			continue
		}
		fieldByJSON[jsonTag] = field

		// Get field descriptions from model documentation
		if description, exists := modelDocs[field.Name]; exists && description != "" {
			propertyDescriptions[jsonTag] = description
			// Debug: log the mapping
			// fmt.Printf("DEBUG: Field %s -> JSON %s -> Description: %s\n", field.Name, jsonTag, description)
		}
	}

	// Process Base model fields
	baseType := reflect.TypeFor[model.Base]()
	for field := range baseType.Fields() {
		jsonTag := getFieldTag(field, consts.TAG_JSON)
		if len(jsonTag) == 0 {
			continue
		}

		// Get field descriptions from Base model documentation
		if description, exists := baseDocs[field.Name]; exists && description != "" {
			propertyDescriptions[jsonTag] = description
		}
	}

	// Add descriptions to schema properties
	for propName, propRef := range schemaRef.Value.Properties {
		if propRef.Value == nil {
			continue
		}

		// Set description if available
		description, exists := propertyDescriptions[propName]
		if !exists || description == "" {
			continue
		}
		if field, ok := fieldByJSON[propName]; ok {
			if updatedSchema := convertDatatypesJSONTypeSchema(propRef, field, description); updatedSchema != nil {
				propRef = updatedSchema
			}
		}
		// Create a copy of the schema to avoid shared reference issues
		if propRef.Value != nil {
			newSchema := *propRef.Value
			newSchema.Title = description
			schemaRef.Value.Properties[propName] = &openapi3.SchemaRef{Value: &newSchema}
		}
	}
}

// convertDatatypesJSONTypeSchema unwraps gorm datatypes.JSONType[T] so the
// generated schema uses the underlying T definition instead of the wrapper.
func convertDatatypesJSONTypeSchema(propRef *openapi3.SchemaRef, field reflect.StructField, description string) *openapi3.SchemaRef {
	if propRef == nil {
		return nil
	}
	typ := field.Type
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.PkgPath() != "gorm.io/datatypes" || (typ.Name() != "JSONType" && !strings.HasPrefix(typ.Name(), "JSONType[")) {
		return propRef
	}

	var dataType reflect.Type
	for f := range typ.Fields() {
		if f.Name == "Data" || f.Name == "data" || f.IsExported() {
			dataType = f.Type
			break
		}
	}

	if dataType == nil {
		return propRef
	}

	value := reflect.Zero(dataType).Interface()

	gen := openapi3gen.NewGenerator()
	schemaRef, err := gen.NewSchemaRefForValue(value, nil)
	if err != nil || schemaRef == nil || schemaRef.Value == nil || (schemaRef.Value.Type == nil && len(schemaRef.Value.Properties) == 0) {
		schemaRef = schemaFromType(dataType)
		if schemaRef == nil {
			zap.S().Warnf("failed to build schema for datatypes.JSONType[%s]: %v", dataType.String(), err)
			return propRef
		}
	}

	if schemaRef.Value != nil && description != "" {
		schemaRef.Value.Title = description
	}

	return schemaRef
}

func schemaFromType(dataType reflect.Type) *openapi3.SchemaRef {
	for dataType.Kind() == reflect.Pointer {
		dataType = dataType.Elem()
	}

	if dataType == timeType {
		return &openapi3.SchemaRef{Value: dateTimeSchema()}
	}

	switch dataType.Kind() {
	case reflect.Struct:
		schema := openapi3.NewObjectSchema()
		for f := range dataType.Fields() {
			if !f.IsExported() {
				continue
			}
			jsonTag := getFieldTag(f, consts.TAG_JSON)
			if jsonTag == "" {
				continue
			}
			schema.WithPropertyRef(jsonTag, &openapi3.SchemaRef{Value: fieldToOpenAPISchema(f)})
		}
		return &openapi3.SchemaRef{Value: schema}
	case reflect.Slice, reflect.Array:
		itemRef := schemaFromType(dataType.Elem())
		if itemRef == nil {
			return nil
		}
		arraySchema := openapi3.NewArraySchema()
		arraySchema.Items = itemRef
		return &openapi3.SchemaRef{Value: arraySchema}
	default:
		return &openapi3.SchemaRef{Value: fieldToOpenAPISchema(reflect.StructField{Type: dataType})}
	}
}

// addQueryParameters adds query parameters for List operation.
func addQueryParameters[M types.Model, REQ types.Request, RSP types.Response](op *openapi3.Operation) {
	// 只有使用默认的逻辑才支持通过结构体字段过滤
	if !modelregistry.AreTypesEqual[M, REQ, RSP]() {
		return
	}

	queries := make([]*openapi3.ParameterRef, 0)

	// Get model field descriptions
	modelInstance := *new(M)
	modelDocs := parseModelDocs(modelInstance)

	typ := reflect.TypeOf(*new(M)).Elem()
	for field := range typ.Fields() {
		// Only fields with query tags are exposed as request query parameters.
		queryTag := getFieldTag(field, consts.TAG_QUERY)
		if len(queryTag) == 0 {
			continue
		}

		// Get field descriptions from model documentation
		description := modelDocs[field.Name]

		queries = append(queries, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        queryTag,
				In:          "query",
				Required:    false,
				Schema:      &openapi3.SchemaRef{Value: fieldToOpenAPISchema(field)},
				Description: description,
			},
		})
	}

	// Get field descriptions of model.Base (using cache)
	baseDocs := getBaseModelDocs()

	baseType := reflect.TypeFor[model.Base]()
	for field := range baseType.Fields() {
		queryTag := getFieldTag(field, consts.TAG_QUERY)
		if len(queryTag) == 0 {
			continue
		}

		// Get field descriptions from Base model documentation
		description := baseDocs[field.Name]

		queries = append(queries, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        queryTag,
				In:          "query",
				Required:    false,
				Schema:      &openapi3.SchemaRef{Value: fieldToOpenAPISchema(field)},
				Description: description,
			},
		})
	}

	// queries := []*openapi3.ParameterRef{
	// 	{
	// 		Value: &openapi3.Parameter{
	// 			Name:     "page",
	// 			In:       "query",
	// 			Required: false,
	// 			Schema: &openapi3.SchemaRef{
	// 				Value: &openapi3.Schema{
	// 					Type:    &openapi3.Types{openapi3.TypeInteger},
	// 					Default: 1,
	// 				},
	// 			},
	// 			Description: "Page number",
	// 		},
	// 	},
	// 	{
	// 		Value: &openapi3.Parameter{
	// 			Name:     "size",
	// 			In:       "query",
	// 			Required: false,
	// 			Schema: &openapi3.SchemaRef{
	// 				Value: &openapi3.Schema{
	// 					Type:    &openapi3.Types{openapi3.TypeInteger},
	// 					Default: 10,
	// 				},
	// 			},
	// 			Description: "Number of items per page",
	// 		},
	// 	},
	// }

	// Avoid duplicate additions
	existing := map[string]bool{}
	for _, p := range op.Parameters {
		if p.Value != nil {
			existing[p.Value.Name] = true
		}
	}

	for _, query := range queries {
		if query.Value != nil && !existing[query.Value.Name] {
			op.Parameters = append(op.Parameters, query)
		}
	}
}

func operationID(op consts.HTTPVerb, typ reflect.Type) string {
	return fmt.Sprintf("%s%s", op, typ.Elem().Name())
}

func summary(path string, op consts.HTTPVerb, _ reflect.Type) string {
	path = strings.TrimPrefix(path, `/api/`)
	path = strings.TrimSuffix(path, `/{id}`)
	items := strings.Split(path, `/`)

	if len(items) > 1 { // trim the first segment
		items = items[1:]
	}

	// remove the segment that starts with ":" or wrapped with {}
	filtered := make([]string, 0, len(items))
	for _, seg := range items {
		if seg == "" || strings.HasPrefix(seg, ":") {
			continue
		}
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			continue
		}
		filtered = append(filtered, seg)
	}

	path = strings.Join(filtered, `/`)
	return strings.ReplaceAll(path, `/`, `_`) + "_" + string(op)

	// // Try to get struct comment first
	// var modelInstance any
	// var elementType reflect.Type
	// if typ.Kind() == reflect.Slice {
	// 	// For slice types, create an instance of the element type
	// 	elementType = typ.Elem()
	// 	modelInstance = reflect.New(elementType).Interface()
	// } else {
	// 	// For other types, create an instance directly
	// 	elementType = typ
	// 	modelInstance = reflect.New(typ).Interface()
	// }
	//
	// structComment := parseStructComment(modelInstance)
	// if structComment != "" {
	// 	return structComment
	// }
	//
	// // Dereference pointer types to get the actual struct type name
	// actualType := elementType
	// for actualType.Kind() == reflect.Pointer {
	// 	actualType = actualType.Elem()
	// }
	//
	// // Fallback to original logic if no struct comment found
	// switch op {
	// case consts.List, consts.CreateMany, consts.DeleteMany, consts.UpdateMany, consts.PatchMany:
	// 	return fmt.Sprintf("%s %s", op, pluralizeCli.Plural(actualType.Name()))
	// }
	// return fmt.Sprintf("%s %s", op, actualType.Name())
}

func description(op consts.HTTPVerb, typ reflect.Type) string {
	// Try to get struct comment first
	var modelInstance any
	if typ.Kind() == reflect.Slice {
		// For slice types, create an instance of the element type
		modelInstance = reflect.New(typ.Elem()).Interface()
	} else {
		// For other types, create an instance directly
		modelInstance = reflect.New(typ).Interface()
	}

	structComment := parseStructComment(modelInstance)
	if structComment != "" {
		return structComment
	}

	// Fallback to original logic if no struct comment found
	switch op {
	case consts.List, consts.CreateMany, consts.DeleteMany, consts.UpdateMany, consts.PatchMany:
		return fmt.Sprintf("%s %s", op, pluralizeCli.Plural(typ.Elem().Name()))
	}
	return fmt.Sprintf("%s %s", op, typ.Elem().Name())
}

func tags(path string, _ consts.HTTPVerb, typ reflect.Type) []string {
	// return []string{typ.Elem().Name()}
	tag := strings.TrimPrefix(path, `/api/`)
	tag = strings.TrimSuffix(tag, `/batch`)
	items := strings.Split(tag, `/`)
	if len(items) > 0 {
		tag = items[0]
	} else {
		tag = typ.Elem().Name()
	}
	return []string{tag}
}

// setupBatchExample will remove field "created_at", "created_by", "updated_at", "updated_by"
//
// Before:
//
//	{
//	  "items": [
//	    {
//	      "created_at": "2025-04-19T19:22:25.166Z",
//	      "created_by": "string",
//	      "desc": "string",
//	      "id": "string",
//	      "member_count": 0,
//	      "name": "string",
//	      "updated_at": "2025-04-19T19:22:25.166Z",
//	      "updated_by": "string"
//	    }
//	  ]
//	}
//
// After:
//
//	{
//	  "items": [
//	    {
//	      "desc": "string",
//	      "id": "string",
//	      "member_count": 0,
//	      "name": "string",
//	    }
//	  ]
//	}

func fieldType2openapiType(field reflect.StructField) *openapi3.Types {
	typ := field.Type

	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.String:
		return &openapi3.Types{openapi3.TypeString}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &openapi3.Types{openapi3.TypeInteger}
	case reflect.Float32, reflect.Float64:
		return &openapi3.Types{openapi3.TypeNumber}
	case reflect.Bool:
		return &openapi3.Types{openapi3.TypeBoolean}
	case reflect.Array:
		return &openapi3.Types{openapi3.TypeArray}
	case reflect.Struct:
		// fmt.Println("----- field name", field.Name, field.Type.Kind())
		return &openapi3.Types{openapi3.TypeObject}
	default:
		// fmt.Println("----- field name", field.Name, field.Type.Kind())
		return &openapi3.Types{openapi3.TypeNull}
	}
}

func fieldToOpenAPISchema(field reflect.StructField) *openapi3.Schema {
	typ := field.Type
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	if typ == timeType {
		return dateTimeSchema()
	}

	return &openapi3.Schema{Type: fieldType2openapiType(field)}
}

func dateTimeSchema() *openapi3.Schema {
	return &openapi3.Schema{
		Type:   &openapi3.Types{openapi3.TypeString},
		Format: "date-time",
	}
}

func newRequestBody[REQ types.Request](reqKey string) *openapi3.RequestBodyRef {
	if modelregistry.IsEmpty[REQ]() {
		return nil
	}
	return &openapi3.RequestBodyRef{
		Ref: "#/components/requestBodies/" + reqKey,
	}
}

func newResponses[RSP types.Response](status int, rspKey string) *openapi3.Responses {
	if modelregistry.IsEmpty[RSP]() {
		return nil
	}
	return openapi3.NewResponses(openapi3.WithStatus(status, &openapi3.ResponseRef{Ref: "#/components/responses/" + rspKey}))
}

// func NewResponses() *openapi3.Responses {
// 	if len(opts) == 0 {
// 		return NewResponses(WithName("default", NewResponse().WithDescription("")))
// 	}
// 	return NewResponses(openapi3.WithName())
// }

type apiBatchRequest[T any] struct {
	Items []T `json:"items"`
}

type apiResponse[T any] struct {
	Code    int    `json:"code"`
	Data    T      `json:"data"`
	Msg     string `json:"msg"`
	TraceID string `json:"trace_id"`
}

type apiListResponse[T any] struct {
	Code    int         `json:"code"`
	Data    listData[T] `json:"data"`
	Msg     string      `json:"msg"`
	TraceID string      `json:"trace_id"`
}
type listData[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

type apiBatchResponse[T any] struct {
	Code    int          `json:"code"`
	Data    batchData[T] `json:"data"`
	Msg     string       `json:"msg"`
	TraceID string       `json:"trace_id"`
}
type listSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

type batchData[T any] struct {
	Items   []T            `json:"items"`
	Options map[string]any `json:"options"`
	Summary listSummary    `json:"summary"`
}

// parameters:
//   - name: limit
//     in: query
//     required: false
//     schema:
//       type: integer
//
//   - name: Authorization
//     in: header
//     required: true
//     schema:
//       type: string
//
//   - name: id
//     in: path
//     required: true
//     schema:
//       type: string
//
//   - name: session_id
//     in: cookie
//     required: false
//     schema:
//       type: string
