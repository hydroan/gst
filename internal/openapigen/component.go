package openapigen

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

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
//	myproject/model/sample.Record                    -> sample.Record
//	myproject/model.Item                             -> Item
//	.../gst/internal/model/iam/user.User             -> iam.user.User
//	.../gst/module/iam.LoginReq                      -> module.iam.LoginReq
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
// action, eg. "sample.record_patch". It keys on the payload or response
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
			applyExample(reqSchemaRef)
			applyBatchExample(reqSchemaRef)
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
		}
		docMutex.Unlock()
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
//
// The success status is fixed at 200 rather than taken from the caller: a
// successful request answers 200 whichever action handled it, so a generated
// document that says otherwise would describe a runtime that does not exist.
func newResponses[RSP types.Response](rspKey string) *openapi3.Responses {
	return openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{Ref: "#/components/responses/" + rspKey}))
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
