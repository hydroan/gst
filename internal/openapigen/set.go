package openapigen

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

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

// componentNameOwners tracks which package owns each component base name, so
// same-named types from different packages never share one component entry.
var (
	componentNameMu     sync.Mutex
	componentNameOwners = map[string]string{}
)

// schemaComponentName derives a readable, package-qualified component name
// for a type: the package path segments after the last "/model/" (a type in
// the model root keeps its bare name), otherwise the last two package path
// segments. Examples:
//
//	dice/model/play.Customization                    -> play.Customization
//	dice/model.User                                  -> User
//	.../gst/internal/model/iam/user.User             -> iam.user.User
//	.../gst/module/mfa.TOTPBind                      -> module.mfa.TOTPBind
func schemaComponentName(typ reflect.Type) string {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return schemaComponentNameFromPath(typ.PkgPath(), typ.Name())
}

// schemaComponentNameFromPath implements the naming rule of
// schemaComponentName on a plain package path and type name.
func schemaComponentNameFromPath(pkgPath, name string) string {
	if pkgPath == "" || name == "" {
		return name
	}

	if index := strings.LastIndex(pkgPath, "/model/"); index >= 0 {
		suffix := strings.ReplaceAll(pkgPath[index+len("/model/"):], "/", ".")
		return suffix + "." + name
	}
	if pkgPath == "model" || strings.HasSuffix(pkgPath, "/model") {
		return name
	}

	segments := strings.Split(pkgPath, "/")
	if len(segments) >= 2 {
		segments = segments[len(segments)-2:]
	}
	return strings.Join(segments, ".") + "." + name
}

// uniqueComponentName returns the component name for a type, guaranteeing
// that two different packages never resolve to the same name: the second
// package to claim a name falls back to its fully qualified package path.
func uniqueComponentName(typ reflect.Type) string {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	name := schemaComponentName(typ)
	pkgPath := typ.PkgPath()
	if pkgPath == "" || name == "" {
		return name
	}

	componentNameMu.Lock()
	defer componentNameMu.Unlock()
	owner, taken := componentNameOwners[name]
	if taken && owner != pkgPath {
		qualified := strings.ReplaceAll(pkgPath, "/", ".") + "." + typ.Name()
		zap.S().Warnf("openapi component name %q is owned by package %q, using %q for package %q", name, owner, qualified, pkgPath)
		return qualified
	}
	componentNameOwners[name] = pkgPath
	return name
}

// actionComponentKey returns the requestBodies/responses component key for one
// action, eg. "play.customization_patch". It keys on the payload or response
// type rather than on the model: a model may expose several actions on the same
// phase, eg. two custom POST routes, and keying on the model alone collapses
// them onto one component where only the first one registered survives. The
// phase stays in the key because one type renders differently per phase, eg. a
// list envelope versus a single record. Anonymous types carry no name to key on
// and fall back to the model plus the route path, which is unique per action.
func actionComponentKey(typ, modelTyp reflect.Type, path string, phase any) string {
	if name := uniqueComponentName(typ); name != "" {
		return fmt.Sprintf("%s_%s", strings.ToLower(name), phase)
	}
	return fmt.Sprintf("%s_%s_%s", strings.ToLower(uniqueComponentName(modelTyp)), pathKeySegment(path), phase)
}

// pathKeySegment reduces a route path to a token usable inside a component key,
// eg. "/api/records/{id}/archive" becomes "api_records_id_archive".
func pathKeySegment(path string) string {
	replacer := strings.NewReplacer("/", "_", "{", "", "}", "", ":", "", "-", "_", ".", "_")
	return strings.Trim(strings.ToLower(replacer.Replace(path)), "_")
}

// componentDescriptionName returns the type name shown on a request or response
// component, falling back to the model for anonymous types.
func componentDescriptionName(typ, modelTyp reflect.Type) string {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Name() != "" {
		return typ.Name()
	}
	for modelTyp.Kind() == reflect.Pointer {
		modelTyp = modelTyp.Elem()
	}
	return modelTyp.Name()
}

func setCreate[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_CREATE)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_CREATE)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	rspSchemaRef := newSchemaRefWithDocs(*new(apiResponse[RSP]))
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)
	successStatus := 201
	if !modelregistry.AreTypesEqual[M, REQ, RSP]() {
		successStatus = 200
	}

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
		OperationID: operationID(path, consts.Create),
		Summary:     summary(path, consts.Create, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Create, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.Create, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](successStatus, rspKey),
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
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_DELETE)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_DELETE)
	rspSchemaRef := newSchemaRefWithDocs(*new(apiResponse[RSP]))
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	pathItem.Delete = &openapi3.Operation{
		OperationID: operationID(path, consts.Delete),
		Summary:     summary(path, consts.Delete, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Delete, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.Delete, typ),
		Parameters:  parseParametersFromPath(path),
		Responses:   newResponses[RSP](200, rspKey),
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
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_UPDATE)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_UPDATE)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	rspSchemaRef := newSchemaRefWithDocs(*new(apiResponse[RSP]))
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Put = &openapi3.Operation{
		OperationID: operationID(path, consts.Update),
		Summary:     summary(path, consts.Update, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Update, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
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
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_PATCH)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_PATCH)
	reqSchemaRef := newSchemaRefWithDocs(*new(REQ))
	rspSchemaRef := newSchemaRefWithDocs(*new(apiResponse[RSP]))
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Patch = &openapi3.Operation{
		OperationID: operationID(path, consts.Patch),
		Summary:     summary(path, consts.Patch, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Patch, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
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
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_LIST)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_LIST)

	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		rspSchemaRef = newSchemaRefWithDocs(*new(apiListResponse[M]))
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
		rspSchemaRef = newSchemaRefWithDocs(*new(apiResponse[RSP]))
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		addSchemaTitle[RSP](dataProperty)
		// 	}
		// }
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	pathItem.Get = &openapi3.Operation{
		OperationID: operationID(path, consts.List),
		Summary:     summary(path, consts.List, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.List, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.List, typ),
		Parameters:  parseParametersFromPath(path),
		Responses:   newResponses[RSP](200, rspKey),
		// // Parameters: []*openapi3.ParameterRef{
		// // 	{
		// // 		Value: &openapi3.Parameter{
		// // 			Name:     "_page",
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
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_GET)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_GET)
	rspSchemaRef := newSchemaRefWithDocs(*new(apiResponse[RSP]))
	registerSchema[M, REQ, RSP](reqKey, rspKey, nil, rspSchemaRef)

	pathItem.Get = &openapi3.Operation{
		OperationID: operationID(path, consts.Get),
		Summary:     summary(path, consts.Get, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Get, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
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
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_CREATE_MANY)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_CREATE_MANY)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		reqSchemaRef = newSchemaRefWithDocs(*new(apiBatchRequest[REQ]))
		// if reqSchemaRef.Value != nil && reqSchemaRef.Value.Properties != nil {
		// 	if itemsProperty, exists := reqSchemaRef.Value.Properties["items"]; exists && itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 		addSchemaTitle[M](itemsProperty.Value.Items)
		// 	}
		// }
		rspSchemaRef = newSchemaRefWithDocs(*new(apiBatchResponse[RSP]))
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
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(*new(apiResponse[RSP]))
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		addSchemaTitle[RSP](dataProperty)
		// 	}
		// }
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)
	successStatus := 201
	if !modelregistry.AreTypesEqual[M, REQ, RSP]() {
		successStatus = 200
	}

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
		OperationID: operationID(path, consts.CreateMany),
		Summary:     summary(path, consts.CreateMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.CreateMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.CreateMany, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](successStatus, rspKey),
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
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_DELETE_MANY)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_DELETE_MANY)
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
		rspSchemaRef = newSchemaRefWithDocs(*new(apiBatchResponse[RSP]))
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists && dataProperty.Value != nil && dataProperty.Value.Properties != nil {
		// 		if itemsProperty, exists := dataProperty.Value.Properties["items"]; exists && itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 			addSchemaTitle[RSP](itemsProperty.Value.Items)
		// 		}
		// 	}
		// }
	} else {
		rspSchemaRef = newSchemaRefWithDocs(*new(apiResponse[RSP]))
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		addSchemaTitle[RSP](dataProperty)
		// 	}
		// }
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Delete = &openapi3.Operation{
		OperationID: operationID(path, consts.DeleteMany),
		Summary:     summary(path, consts.DeleteMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.DeleteMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.DeleteMany, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: newRequestBody[REQ](reqKey),
		Responses:   newResponses[RSP](200, rspKey),
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
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_UPDATE_MANY)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_UPDATE_MANY)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		reqSchemaRef = newSchemaRefWithDocs(*new(apiBatchRequest[REQ]))
		// if reqSchemaRef.Value != nil && reqSchemaRef.Value.Properties != nil {
		// 	if itemsProperty, exists := reqSchemaRef.Value.Properties["items"]; exists && itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 		addSchemaTitle[M](itemsProperty.Value.Items)
		// 	}
		// }
		rspSchemaRef = newSchemaRefWithDocs(*new(apiBatchResponse[REQ]))
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
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(*new(apiResponse[RSP]))
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		addSchemaTitle[RSP](dataProperty)
		// 	}
		// }
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Put = &openapi3.Operation{
		OperationID: operationID(path, consts.UpdateMany),
		Summary:     summary(path, consts.UpdateMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.UpdateMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
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
	typ := reflect.TypeOf(*new(M))
	reqKey := actionComponentKey(reflect.TypeOf(*new(REQ)), typ, path, consts.PHASE_PATCH_MANY)
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_PATCH_MANY)

	var reqSchemaRef *openapi3.SchemaRef
	var rspSchemaRef *openapi3.SchemaRef
	if modelregistry.AreTypesEqual[M, REQ, RSP]() {
		reqSchemaRef = newSchemaRefWithDocs(*new(apiBatchRequest[REQ]))
		// if reqSchemaRef.Value != nil && reqSchemaRef.Value.Properties != nil {
		// 	if itemsProperty, exists := reqSchemaRef.Value.Properties["items"]; exists && itemsProperty.Value != nil && itemsProperty.Value.Items != nil {
		// 		addSchemaTitle[M](itemsProperty.Value.Items)
		// 	}
		// }
		rspSchemaRef = newSchemaRefWithDocs(*new(apiBatchResponse[RSP]))
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
		reqSchemaRef = newSchemaRefWithDocs(*new(REQ))
		rspSchemaRef = newSchemaRefWithDocs(*new(apiResponse[RSP]))
		// if rspSchemaRef.Value != nil && rspSchemaRef.Value.Properties != nil {
		// 	if dataProperty, exists := rspSchemaRef.Value.Properties["data"]; exists {
		// 		addSchemaTitle[RSP](dataProperty)
		// 	}
		// }
	}
	registerSchema[M, REQ, RSP](reqKey, rspKey, reqSchemaRef, rspSchemaRef)

	pathItem.Patch = &openapi3.Operation{
		OperationID: operationID(path, consts.PatchMany),
		Summary:     summary(path, consts.PatchMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.PatchMany, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
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

// Media types and file-format query values documented for the import/export
// operations. They mirror the values produced by the import/export controllers:
// export streams a csv or xlsx file, and import reads a multipart file upload.
const (
	exportFormatCSV  = "csv"
	exportFormatXLSX = "xlsx"

	exportMediaTypeCSV  = "text/csv"
	exportMediaTypeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

	// importFileField is the multipart form field name read by the import
	// controller (c.FormFile("file")); the request body schema must match it.
	importFileField = "file"
)

// setImport documents the import action (POST /{path}/import). The controller
// reads a single uploaded file, so the request body is a multipart/form-data
// upload with a required binary "file" field, and the response reuses the
// standard success envelope returned by the controller.
func setImport[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))
	rspKey := actionComponentKey(reflect.TypeOf(*new(RSP)), typ, path, consts.PHASE_IMPORT)
	rspSchemaRef := newSchemaRefWithDocs(*new(apiResponse[RSP]))
	registerSchema[M, REQ, RSP](rspKey, rspKey, nil, rspSchemaRef)

	pathItem.Post = &openapi3.Operation{
		OperationID: operationID(path, consts.Import),
		Summary:     summary(path, consts.Import, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Import, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.Import, typ),
		Parameters:  parseParametersFromPath(path),
		RequestBody: importFileRequestBody(),
		Responses:   newResponses[RSP](200, rspKey),
	}
	addHeaderParameters(pathItem.Post)
}

// importFileRequestBody documents the multipart/form-data upload consumed by
// the import controller: a single required binary file field.
func importFileRequestBody() *openapi3.RequestBodyRef {
	return &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Description: "The file to import.",
			Required:    true,
			Content: openapi3.Content{
				"multipart/form-data": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type: &openapi3.Types{openapi3.TypeObject},
							Properties: openapi3.Schemas{
								importFileField: {
									Value: &openapi3.Schema{
										Type:   &openapi3.Types{openapi3.TypeString},
										Format: "binary",
									},
								},
							},
							Required: []string{importFileField},
						},
					},
				},
			},
		},
	}
}

// setExport documents the export action (GET /{path}/export). The controller
// filters resources with the same query parameters as the list action and
// streams the result as a downloadable file, so the operation carries the list
// filters plus the file-format selector and a binary file response.
func setExport[M types.Model, REQ types.Request, RSP types.Response](path string, pathItem *openapi3.PathItem) {
	typ := reflect.TypeOf(*new(M))

	pathItem.Get = &openapi3.Operation{
		OperationID: operationID(path, consts.Export),
		Summary:     summary(path, consts.Export, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Description: description(path, consts.Export, typ, !modelregistry.AreTypesEqual[M, REQ, RSP]()),
		Tags:        tags(path, consts.Export, typ),
		Parameters:  append(parseParametersFromPath(path), exportFormatParameter()),
		Responses:   exportFileResponses(),
	}
	addQueryParameters[M, REQ, RSP](pathItem.Get)
	addHeaderParameters(pathItem.Get)
}

// exportFormatParameter documents the file-format query parameter accepted by
// the export controller, restricting the value to the supported formats.
func exportFormatParameter() *openapi3.ParameterRef {
	return &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:        consts.QUERY_FORMAT,
			In:          "query",
			Required:    false,
			Description: "The export file format.",
			Schema: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{openapi3.TypeString},
					Enum: []any{exportFormatCSV, exportFormatXLSX},
				},
			},
		},
	}
}

// exportFileResponses documents the export download: a 200 response whose body
// is the generated csv or xlsx file delivered as a binary stream.
func exportFileResponses() *openapi3.Responses {
	fileSchema := &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:   &openapi3.Types{openapi3.TypeString},
			Format: "binary",
		},
	}
	response := openapi3.NewResponse().
		WithDescription("The exported file.").
		WithContent(openapi3.Content{
			exportMediaTypeCSV:  openapi3.NewMediaType().WithSchemaRef(fileSchema),
			exportMediaTypeXLSX: openapi3.NewMediaType().WithSchemaRef(fileSchema),
		})
	return openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{Value: response}))
}

// register Model, Model Payload, Model Result into openapi3 schema.
func registerSchema[M types.Model, REQ types.Request, RSP types.Response](reqKey, rspKey string, reqSchemaRef *openapi3.SchemaRef, rspSchemaRef *openapi3.SchemaRef) {
	if !modelregistry.IsEmpty[M]() {
		typ := reflect.TypeOf(*new(M))
		name := uniqueComponentName(typ)
		docMutex.Lock()
		if doc.Components.Schemas == nil {
			doc.Components.Schemas = openapi3.Schemas{}
		}
		if _, ok := doc.Components.Schemas[name]; !ok {
			if schemaRef := newSchemaRefWithDocs(*new(M)); schemaRef != nil {
				doc.Components.Schemas[name] = schemaRef
			}
		}
		docMutex.Unlock()
	}

	if !modelregistry.IsEmpty[REQ]() {
		name := componentDescriptionName(reflect.TypeOf(*new(REQ)), reflect.TypeOf(*new(M)))

		docMutex.Lock()
		if doc.Components.RequestBodies == nil {
			doc.Components.RequestBodies = openapi3.RequestBodies{}
		}
		if _, ok := doc.Components.RequestBodies[reqKey]; !ok && reqSchemaRef != nil {
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

	{
		name := componentDescriptionName(reflect.TypeOf(*new(RSP)), reflect.TypeOf(*new(M)))
		if modelregistry.IsEmpty[RSP]() {
			markEmptyResponseData(rspSchemaRef)
		}

		docMutex.Lock()
		if doc.Components.Responses == nil {
			doc.Components.Responses = openapi3.ResponseBodies{}
		}
		if _, ok := doc.Components.Responses[rspKey]; !ok && rspSchemaRef != nil {
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

// setupExample builds the request body example and removes the Base auto
// fields ("created_at", "created_by", "updated_at", "updated_by", "id") from
// the top level only. Nested structs keep those property names because there
// they are caller-supplied fields rather than Base auto fields.
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

// isRefOrMissing reports whether schemaRef carries no usable inline schema to
// build an example from.
//
// A member rendered as a $ref keeps an inline Value in memory, but that Value
// is dropped on serialization and, being reached through a cycle, never got
// decorated with the enum values and formats its component carries. Building an
// example from it invents values the referenced component rejects, so the
// descent stops at the boundary and readers follow the $ref instead.
func isRefOrMissing(schemaRef *openapi3.SchemaRef) bool {
	return schemaRef == nil || schemaRef.Ref != "" || schemaRef.Value == nil
}

// exampleForStringFormat returns the example value of a formatted string. A
// bare "string" placeholder is rejected by validators for these formats, since
// the format carries its own pattern.
func exampleForStringFormat(format string) (string, bool) {
	switch format {
	case "date-time":
		return "2006-01-02T15:04:05Z", true
	case "date":
		return "2006-01-02", true
	default:
		return "", false
	}
}

// buildExampleValue recursively builds an example value for schema so nested
// arrays, structs, and maps (additionalProperties) show their full shape in
// Swagger instead of an empty placeholder.
//
// A self-referential type recurses until maxExampleDepth stops it. The descent
// stops with an empty array or object rather than with a null, because the
// member being filled is typically not nullable and a null there makes the
// example fail validation against the very schema it illustrates.
func buildExampleValue(schema *openapi3.Schema, depth int) any {
	if schema == nil {
		return nil
	}

	// An enum accepts nothing but its declared values, whatever its JSON type.
	if len(schema.Enum) > 0 {
		return schema.Enum[0]
	}

	// A schema without a type accepts any value, eg. the value side of a
	// user-defined JSON map. Only an explicitly nullable member may stay null,
	// so illustrate the rest with a string.
	if schema.Type == nil {
		if schema.Nullable {
			return nil
		}
		return "string"
	}

	switch {
	case schema.Type.Is(openapi3.TypeString):
		if example, ok := exampleForStringFormat(schema.Format); ok {
			return example
		}
		return "string"
	case schema.Type.Is(openapi3.TypeInteger):
		return 0
	case schema.Type.Is(openapi3.TypeNumber):
		return 0.0
	case schema.Type.Is(openapi3.TypeBoolean):
		return false
	case schema.Type.Is(openapi3.TypeArray):
		if isRefOrMissing(schema.Items) || depth >= maxExampleDepth {
			return []any{}
		}
		return []any{buildExampleValue(schema.Items.Value, depth+1)}
	case schema.Type.Is(openapi3.TypeObject):
		if depth >= maxExampleDepth {
			return map[string]any{}
		}
		if len(schema.Properties) > 0 {
			example := make(map[string]any, len(schema.Properties))
			for propName, propRef := range schema.Properties {
				if isRefOrMissing(propRef) {
					continue
				}
				// Nested fields keep their id/audit-named properties: at this
				// depth they are caller-supplied fields, not the Base auto
				// fields that only appear at the request top level.
				example[propName] = buildExampleValue(propRef.Value, depth+1)
			}
			return example
		}
		if !isRefOrMissing(schema.AdditionalProperties.Schema) {
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

// openAPIDocComment removes the Go doc subject from API-facing text while
// preserving comments that do not begin with the exact declared symbol.
func openAPIDocComment(symbol, comment string) string {
	comment = strings.TrimSpace(comment)
	if symbol == "" || comment == "" {
		return comment
	}

	rest, ok := strings.CutPrefix(comment, symbol)
	if !ok || rest == "" {
		return comment
	}

	switch {
	case strings.HasPrefix(rest, ":"):
		rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
	case strings.HasPrefix(rest, "："):
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "："))
	default:
		boundary, _ := utf8.DecodeRuneInString(rest)
		if !unicode.IsSpace(boundary) {
			return comment
		}
		rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
		switch {
		case strings.HasPrefix(rest, ":"):
			rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
		case strings.HasPrefix(rest, "："):
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "："))
		}
	}

	if rest == "" {
		return comment
	}
	switch {
	case strings.HasPrefix(rest, "是否"):
	case strings.HasPrefix(rest, "是"):
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "是"))
	default:
		if trimmed, found := trimOpenAPIDocCopula(rest, "is"); found {
			rest = trimmed
		} else if trimmed, found := trimOpenAPIDocCopula(rest, "are"); found {
			rest = trimmed
		}
	}

	if rest == "" {
		return comment
	}
	first, size := utf8.DecodeRuneInString(rest)
	if upper := unicode.ToUpper(first); upper != first {
		rest = string(upper) + rest[size:]
	}
	return rest
}

func trimOpenAPIDocCopula(comment, copula string) (string, bool) {
	rest, ok := strings.CutPrefix(comment, copula)
	if !ok || rest == "" {
		return comment, false
	}
	boundary, _ := utf8.DecodeRuneInString(rest)
	if !unicode.IsSpace(boundary) {
		return comment, false
	}
	return strings.TrimLeftFunc(rest, unicode.IsSpace), true
}

// newSchemaRefWithDocs generates the OpenAPI schema for value and decorates it
// and every nested schema with the doc comments and enum values registered for
// the Go types reachable from value's type.
//
// A self-referential type, eg. a tree node holding its own children, cannot be
// inlined forever, so the generator breaks the cycle by emitting a $ref into
// components. Those $ref targets are named through uniqueComponentName, the
// same rule registerSchema names components with, otherwise the $ref points at
// a component that was never registered and the whole document fails to load.
func newSchemaRefWithDocs(value any) *openapi3.SchemaRef {
	schemaRef, err := openapi3gen.NewSchemaRefForValue(value, nil, openapi3gen.CreateTypeNameGenerator(uniqueComponentName))
	if err != nil {
		return schemaRef
	}
	addSchemaDocsForType(reflect.TypeOf(value), schemaRef, nil)
	return schemaRef
}

// addSchemaDocsForType decorates schemaRef with the doc comments and enum
// values registered for typ. Field comments become descriptions without being
// copied into titles, so documentation renderers do not display the same text
// twice and independent schema titles remain intact. It walks the generated
// schema tree and the Go type tree in parallel, so nested request and response
// structs are decorated at every depth. visiting holds the struct types on the
// current descent path so self-referential types terminate; callers pass nil.
func addSchemaDocsForType(typ reflect.Type, schemaRef *openapi3.SchemaRef, visiting map[reflect.Type]bool) {
	if typ == nil || schemaRef == nil || schemaRef.Value == nil {
		return
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.Slice, reflect.Array:
		addSchemaDocsForType(typ.Elem(), schemaRef.Value.Items, visiting)
		return
	case reflect.Struct:
	default:
		return
	}
	if typ == timeType || len(schemaRef.Value.Properties) == 0 {
		return
	}
	if visiting[typ] {
		return
	}
	if visiting == nil {
		visiting = make(map[reflect.Type]bool)
	}
	visiting[typ] = true
	defer delete(visiting, typ)

	fields := make(map[string]schemaDocField)
	collectSchemaDocFields(typ, fields, make(map[reflect.Type]bool))

	for propName, propRef := range schemaRef.Value.Properties {
		if propRef == nil || propRef.Value == nil {
			continue
		}
		docField, hasField := fields[propName]
		if !hasField {
			continue
		}

		description := openAPIDocComment(docField.field.Name, docField.docs[docField.field.Name])

		// Unwrap gorm datatypes.JSONType[T] so both the schema and the type
		// walk below continue with the wrapped data type.
		fieldType := docField.field.Type
		if dataType, isJSONType := datatypesJSONDataType(fieldType); isJSONType {
			if unwrapped := convertDatatypesJSONTypeSchema(propRef, docField.field); unwrapped != nil {
				propRef = unwrapped
				schemaRef.Value.Properties[propName] = propRef
			}
			fieldType = dataType
		}

		enumDoc, enumOnItems, hasEnum := fieldEnumDoc(fieldType)
		if (description != "" || hasEnum) && propRef.Value != nil {
			// Copy the schema so shared schema instances keep their own docs.
			newSchema := *propRef.Value
			newSchema.Description = description
			if hasEnum {
				applyEnum(&newSchema, enumOnItems, enumDoc)
				newSchema.Description = enumDescription(description, enumDoc)
			}
			propRef = &openapi3.SchemaRef{Value: &newSchema}
			schemaRef.Value.Properties[propName] = propRef
		}

		addSchemaDocsForType(fieldType, propRef, visiting)
	}
}

// schemaDocField pairs one JSON-visible struct field with the doc comments of
// the struct that declares it.
type schemaDocField struct {
	field reflect.StructField
	docs  map[string]string
}

// collectSchemaDocFields maps every JSON property name of typ to its declaring
// struct field and that struct's doc comments, descending into anonymous
// embedded structs the same way encoding/json promotes fields. Fields already
// collected win over deeper promoted fields, matching encoding/json
// shallow-first precedence. visited stops self-embedding chains.
func collectSchemaDocFields(typ reflect.Type, fields map[string]schemaDocField, visited map[reflect.Type]bool) {
	if visited[typ] {
		return
	}
	visited[typ] = true

	docs := parseModelDocs(reflect.New(typ).Interface())

	var embedded []reflect.Type
	for field := range typ.Fields() {
		jsonTag := getFieldTag(field, consts.TAG_JSON)
		if field.Anonymous && jsonTag == "" {
			embeddedType := field.Type
			for embeddedType.Kind() == reflect.Pointer {
				embeddedType = embeddedType.Elem()
			}
			if embeddedType.Kind() == reflect.Struct {
				embedded = append(embedded, embeddedType)
			}
			continue
		}
		if jsonTag == "" {
			continue
		}
		if _, exists := fields[jsonTag]; exists {
			continue
		}
		fields[jsonTag] = schemaDocField{field: field, docs: docs}
	}

	for _, embeddedType := range embedded {
		collectSchemaDocFields(embeddedType, fields, visited)
	}
}

// collectQueryDocFields collects query-tagged fields from typ and anonymous
// embedded structs in breadth-first order. Fields declared at a shallower
// depth win over deeper promoted fields with the same query name, matching Go
// field selection precedence.
func collectQueryDocFields(typ reflect.Type) []schemaDocField {
	if typ == nil {
		return nil
	}

	fields := make([]schemaDocField, 0)
	seen := make(map[string]bool)
	visited := make(map[reflect.Type]bool)
	queue := []reflect.Type{typ}
	for len(queue) > 0 {
		next := make([]reflect.Type, 0)
		for _, currentType := range queue {
			for currentType.Kind() == reflect.Pointer {
				currentType = currentType.Elem()
			}
			if currentType.Kind() != reflect.Struct || visited[currentType] {
				continue
			}
			visited[currentType] = true

			docs := parseModelDocs(reflect.New(currentType).Interface())
			for field := range currentType.Fields() {
				queryTag := getFieldTag(field, consts.TAG_QUERY)
				if queryTag != "" {
					if !seen[queryTag] {
						seen[queryTag] = true
						fields = append(fields, schemaDocField{field: field, docs: docs})
					}
					continue
				}
				if !field.Anonymous {
					continue
				}

				embeddedType := field.Type
				for embeddedType.Kind() == reflect.Pointer {
					embeddedType = embeddedType.Elem()
				}
				if embeddedType.Kind() == reflect.Struct {
					next = append(next, embeddedType)
				}
			}
		}
		queue = next
	}
	return fields
}

// fieldEnumDoc resolves the registered enum doc of a field type, unwrapping
// pointers, slices and arrays. The second result reports whether the enum
// applies to the slice items schema instead of the field schema itself.
func fieldEnumDoc(typ reflect.Type) (doc apidoc.EnumDoc, onItems bool, ok bool) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
		onItems = true
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
	}
	if typ.PkgPath() == "" || typ.Name() == "" {
		return apidoc.EnumDoc{}, false, false
	}
	doc, ok = apidoc.LookupEnum(typ.PkgPath(), typ.Name())
	if !ok || len(doc.Values) == 0 {
		return apidoc.EnumDoc{}, false, false
	}
	doc.Comment = openAPIDocComment(typ.Name(), doc.Comment)
	return doc, onItems, true
}

// applyEnum sets the enum values on the property schema, or on a copy of its
// items schema for slice fields to avoid mutating shared item schemas.
func applyEnum(schema *openapi3.Schema, onItems bool, doc apidoc.EnumDoc) {
	values := make([]any, 0, len(doc.Values))
	for _, value := range doc.Values {
		values = append(values, value.Value)
	}

	if !onItems {
		schema.Enum = values
		return
	}
	if schema.Items == nil || schema.Items.Value == nil {
		return
	}
	items := *schema.Items.Value
	items.Enum = values
	schema.Items = &openapi3.SchemaRef{Value: &items}
}

// enumDescription appends the enum value list to the field description so
// each value's comment stays visible next to the field. When the field has
// no comment of its own, the enum type comment is used as the base text.
func enumDescription(base string, doc apidoc.EnumDoc) string {
	if base == "" {
		base = doc.Comment
	}

	lines := make([]string, 0, len(doc.Values))
	for _, value := range doc.Values {
		line := fmt.Sprintf("- `%v`", value.Value)
		if value.Comment != "" {
			line += ": " + value.Comment
		}
		lines = append(lines, line)
	}
	list := strings.Join(lines, "\n")

	if base == "" {
		return list
	}
	return base + "\n\n" + list
}

// datatypesJSONDataType returns the wrapped data type of a gorm
// datatypes.JSONType[T] field type, unwrapping pointers first.
func datatypesJSONDataType(typ reflect.Type) (reflect.Type, bool) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.PkgPath() != "gorm.io/datatypes" || (typ.Name() != "JSONType" && !strings.HasPrefix(typ.Name(), "JSONType[")) {
		return nil, false
	}
	for f := range typ.Fields() {
		if f.Name == "Data" || f.Name == "data" || f.IsExported() {
			return f.Type, true
		}
	}
	return nil, false
}

// convertDatatypesJSONTypeSchema unwraps gorm datatypes.JSONType[T] so the
// generated schema uses the underlying T definition instead of the wrapper.
func convertDatatypesJSONTypeSchema(propRef *openapi3.SchemaRef, field reflect.StructField) *openapi3.SchemaRef {
	if propRef == nil {
		return nil
	}
	dataType, isJSONType := datatypesJSONDataType(field.Type)
	if !isJSONType {
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

	return schemaRef
}

func schemaFromType(dataType reflect.Type) *openapi3.SchemaRef {
	return schemaFromTypeVisiting(dataType, nil)
}

// schemaFromTypeVisiting implements schemaFromType. visiting holds the struct
// types on the current descent path so a self-referential type, eg. a tree node
// holding its own children, terminates instead of recursing forever; callers
// pass nil.
func schemaFromTypeVisiting(dataType reflect.Type, visiting map[reflect.Type]bool) *openapi3.SchemaRef {
	for dataType.Kind() == reflect.Pointer {
		dataType = dataType.Elem()
	}

	if dataType == timeType {
		return &openapi3.SchemaRef{Value: dateTimeSchema()}
	}

	switch dataType.Kind() {
	case reflect.Struct:
		schema := openapi3.NewObjectSchema()
		if visiting[dataType] {
			// Reaching the same struct again closes a cycle: describe it as a
			// bare object rather than descend into it once more.
			return &openapi3.SchemaRef{Value: schema}
		}
		if visiting == nil {
			visiting = make(map[reflect.Type]bool)
		}
		visiting[dataType] = true
		defer delete(visiting, dataType)

		for f := range dataType.Fields() {
			if !f.IsExported() {
				continue
			}
			jsonTag := getFieldTag(f, consts.TAG_JSON)
			if jsonTag == "" {
				continue
			}
			schema.WithPropertyRef(jsonTag, schemaFromTypeVisiting(f.Type, visiting))
		}
		return &openapi3.SchemaRef{Value: schema}
	case reflect.Slice, reflect.Array:
		itemRef := schemaFromTypeVisiting(dataType.Elem(), visiting)
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
	// Model-field query filters are available only to the default CRUD path.
	if !modelregistry.AreTypesEqual[M, REQ, RSP]() {
		return
	}

	typ := reflect.TypeFor[M]()
	fields := collectQueryDocFields(typ)
	m := reflect.New(typ.Elem()).Interface().(types.Model) //nolint:errcheck
	queryable := modelregistry.IsQueryable(m)

	queries := make([]*openapi3.ParameterRef, 0, len(fields))
	for _, docField := range fields {
		field := docField.field
		queryTag := getFieldTag(field, consts.TAG_QUERY)
		description := openAPIDocComment(field.Name, docField.docs[field.Name])
		schemaRef := schemaFromType(field.Type)
		if enumDoc, onItems, ok := fieldEnumDoc(field.Type); ok && schemaRef != nil && schemaRef.Value != nil {
			applyEnum(schemaRef.Value, onItems, enumDoc)
			description = enumDescription(description, enumDoc)
		}
		// Business filter fields on queryable models additionally accept the
		// "field[op]=value" operator filter syntax; framework parameters in
		// the "_" namespace do not.
		if queryable && !strings.HasPrefix(queryTag, "_") {
			description = operatorFilterDescription(description, queryTag)
		}
		// The _expand parameter only accepts the model's expandable
		// association names, so list them where the frontend reads them.
		if queryable && queryTag == consts.QUERY_EXPAND {
			description = expandableFieldsDescription(description, m.Expands())
		}

		queries = append(queries, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        queryTag,
				In:          "query",
				Required:    false,
				Schema:      schemaRef,
				Description: description,
			},
		})
	}

	// Cursor-only models accept _size as the batch size, but the field with
	// its query tag lives in Pagination, so the parameter is synthesized.
	if modelregistry.IsCursorable(m) && !modelregistry.IsPaginatable(m) {
		queries = append(queries, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        consts.QUERY_SIZE,
				In:          "query",
				Required:    false,
				Schema:      &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeInteger}}},
				Description: "Batch size for cursor pagination.",
			},
		})
	}

	// The framework-managed Base/AutoBase timestamps carry query:"-" on the
	// model, so their filter parameters are synthesized: the bare name is an
	// exact-match filter like every other documented parameter, and the
	// operator syntax covers ranges.
	if queryable && embedsBaseModel(typ.Elem()) {
		for column, doc := range map[string]string{
			"created_at": "record creation time",
			"updated_at": "record last update time",
		} {
			queries = append(queries, &openapi3.ParameterRef{
				Value: &openapi3.Parameter{
					Name:        column,
					In:          "query",
					Required:    false,
					Schema:      &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
					Description: "Exact-match filter for the " + doc + ".\n\nOperator filter: " + column + "[op]=value, op: eq/ne/gt/gte/lt/lte/isnull; range example: " + column + "[gte]=2026-07-01&" + column + "[lte]=2026-07-15.",
				},
			})
		}
	}

	// Business filter columns always come first; framework parameters keep a
	// canonical trailing order regardless of where the framework structs are
	// embedded in the model.
	sortQueryParameters(queries)

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
			existing[query.Value.Name] = true
		}
	}
}

// embedsBaseModel reports whether the model struct embeds Base or AutoBase.
func embedsBaseModel(typ reflect.Type) bool {
	if typ.Kind() != reflect.Struct {
		return false
	}
	for _, name := range []string{"Base", "AutoBase"} {
		if field, ok := typ.FieldByName(name); ok && field.Anonymous {
			return true
		}
	}
	return false
}

// frameworkQueryParameterOrder is the canonical trailing order of framework
// query parameters in generated API documents, sorted by how commonly each
// parameter is used: pagination on every list page, then table sorting, then
// cursor pagination (the primary paging style for large datasets), then
// association expansion (meaningful only on models that declare expandable
// associations), and finally the unsafe advanced controls with the most
// obscure ones last.
var frameworkQueryParameterOrder = []string{
	consts.QUERY_PAGE, consts.QUERY_SIZE, consts.QUERY_SORT_BY,
	consts.QUERY_CURSOR_VALUE, consts.QUERY_CURSOR_FIELD, consts.QUERY_CURSOR_NEXT,
	consts.QUERY_EXPAND, consts.QUERY_DEPTH,
	consts.QUERY_OR, consts.QUERY_SELECT, consts.QUERY_NO_TOTAL, consts.QUERY_NO_CACHE, consts.QUERY_INDEX,
}

// sortQueryParameters puts business filter parameters first, preserving their
// collection order, and framework "_" parameters after them in the canonical
// frameworkQueryParameterOrder; framework parameters missing from the
// canonical list keep their relative order at the end.
func sortQueryParameters(queries []*openapi3.ParameterRef) {
	rank := func(name string) int {
		if !strings.HasPrefix(name, "_") {
			return 0
		}
		for i, known := range frameworkQueryParameterOrder {
			if name == known {
				return 1 + i
			}
		}
		return 1 + len(frameworkQueryParameterOrder)
	}
	sort.SliceStable(queries, func(i, j int) bool {
		return rank(queries[i].Value.Name) < rank(queries[j].Value.Name)
	})
}

// expandableFieldsDescription appends the model's expandable association
// names to the _expand parameter description so the frontend can see the
// accepted values.
func expandableFieldsDescription(description string, expands []string) string {
	var note string
	if len(expands) == 0 {
		note = "This model has no expandable associations."
	} else {
		note = "Expandable: " + strings.Join(expands, ", ") + ", or all (snake case accepted, matched case-insensitively)."
	}
	if len(description) == 0 {
		return note
	}
	return description + "\n\n" + note
}

// operatorFilterDescription appends the field operator filter note to a query
// parameter description, listing the operators accepted by the
// "field[op]=value" syntax.
func operatorFilterDescription(description, queryTag string) string {
	ops := types.FilterOps()
	tokens := make([]string, 0, len(ops))
	for _, op := range ops {
		tokens = append(tokens, string(op))
	}
	note := "Operator filter: " + queryTag + "[op]=value, op: " + strings.Join(tokens, "/") + "."
	if len(description) == 0 {
		return note
	}
	return description + "\n\n" + note
}

// operationID derives a unique, stable operation id from the route path and
// the action, eg. PATCH /api/play/customizations/{id} -> "play_customizations_patch".
// Deriving from the path instead of the model name keeps ids unique when
// same-named models exist in different packages or one model serves several
// routes; duplicate operation ids break OpenAPI client generators.
func operationID(path string, op consts.HTTPVerb) string {
	token := strings.Join(resourceSegments(path), "_")
	if token == "" {
		return string(op)
	}
	return strings.ReplaceAll(token, "-", "_") + "_" + string(op)
}

// resourceSegments returns the resource segments of a route path: the /api
// prefix, path parameters and empty segments are dropped.
func resourceSegments(path string) []string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	filtered := make([]string, 0, len(segments))
	for index, seg := range segments {
		if seg == "" || (index == 0 && seg == "api") || strings.HasPrefix(seg, ":") {
			continue
		}
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			continue
		}
		filtered = append(filtered, seg)
	}
	return filtered
}

// operationDocInput assembles the apidoc.Operation describing one route
// operation for the summary/description generators. customTypes reports
// whether the operation declares its own request/response types instead of
// reusing the model (see apidoc.Operation.CustomTypes).
func operationDocInput(path string, verb consts.HTTPVerb, typ reflect.Type, customTypes bool) apidoc.Operation {
	elem := typ
	for elem.Kind() == reflect.Pointer || elem.Kind() == reflect.Slice {
		elem = elem.Elem()
	}
	return apidoc.Operation{
		Method:       verb.HTTPMethod(),
		Path:         path,
		Verb:         verb,
		CustomTypes:  customTypes,
		ModelName:    elem.Name(),
		ModelComment: openAPIStructComment(typ),
	}
}

// summary returns the operation summary: an explicitly registered
// apidoc.OperationDoc wins, otherwise the (replaceable) apidoc.GenerateSummary
// builds it from the verb, path and model doc comment.
func summary(path string, verb consts.HTTPVerb, typ reflect.Type, customTypes bool) string {
	op := operationDocInput(path, verb, typ, customTypes)
	if doc, ok := apidoc.LookupOperation(op.Method, op.Path); ok && doc.Summary != "" {
		return doc.Summary
	}
	if generate := apidoc.GenerateSummary; generate != nil {
		return generate(op)
	}
	return apidoc.DefaultSummary(op)
}

// description returns the operation description: an explicitly registered
// apidoc.OperationDoc wins, otherwise the (replaceable)
// apidoc.GenerateDescription builds it from the model doc comment.
func description(path string, verb consts.HTTPVerb, typ reflect.Type, customTypes bool) string {
	op := operationDocInput(path, verb, typ, customTypes)
	if doc, ok := apidoc.LookupOperation(op.Method, op.Path); ok && doc.Description != "" {
		return doc.Description
	}
	if generate := apidoc.GenerateDescription; generate != nil {
		return generate(op)
	}
	return apidoc.DefaultDescription(op)
}

func openAPIStructComment(typ reflect.Type) string {
	instance := elemInstance(typ)
	_, typeName := typeIdentity(instance)
	return openAPIDocComment(typeName, parseStructComment(instance))
}

// elemInstance creates a model instance for comment parsing, unwrapping
// slice types to their element type.
func elemInstance(typ reflect.Type) any {
	if typ.Kind() == reflect.Slice {
		return reflect.New(typ.Elem()).Interface()
	}
	return reflect.New(typ).Interface()
}

// tags groups an operation under the first resource segment of its path,
// which matches the module structure of the backend (eg. play, groups,
// players). Path parameters never become tags.
func tags(path string, _ consts.HTTPVerb, typ reflect.Type) []string {
	segments := resourceSegments(strings.TrimSuffix(path, `/batch`))
	if len(segments) > 0 {
		return []string{segments[0]}
	}
	return []string{typ.Elem().Name()}
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
	case reflect.Array, reflect.Slice:
		return &openapi3.Types{openapi3.TypeArray}
	case reflect.Struct, reflect.Map:
		return &openapi3.Types{openapi3.TypeObject}
	default:
		// An unmapped kind, eg. an interface, constrains nothing. Leaving the
		// type out says exactly that, whereas "null" is not a type OpenAPI 3.0
		// defines and makes the enclosing schema invalid.
		return nil
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

// newResponses references the response component for one action. Every
// operation declares a response, including actions whose response type carries
// no fields: those still answer with the envelope, and responses is a required
// member of an OpenAPI operation.
func newResponses[RSP types.Response](status int, rspKey string) *openapi3.Responses {
	return openapi3.NewResponses(openapi3.WithStatus(status, &openapi3.ResponseRef{Ref: "#/components/responses/" + rspKey}))
}

// markEmptyResponseData rewrites the data member of an envelope whose response
// type carries no fields. Such an action answers with data set to null, so the
// member records only its nullability rather than an empty object body.
func markEmptyResponseData(schemaRef *openapi3.SchemaRef) {
	if schemaRef == nil || schemaRef.Value == nil {
		return
	}
	data := schemaRef.Value.Properties["data"]
	if data == nil || data.Value == nil {
		return
	}
	data.Value.Type = nil
	data.Value.Properties = nil
	data.Value.Nullable = true
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
