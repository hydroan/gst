package openapigen

import (
	"reflect"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/internal/apiaction"
)

// newOperation documents the operation the route path serves for phase on
// the model type typ, all but its request body and its responses, which the
// caller adds: the id, summary, description and tags the path and the model
// give it, and the parameters the path declares. custom reports whether the
// action declares its own request or response type instead of the model (see
// apidoc.Operation.CustomTypes).
func newOperation(path string, phase apiaction.Phase, typ reflect.Type, custom bool) *openapi3.Operation {
	return &openapi3.Operation{
		OperationID: operationID(path, phase),
		Summary:     summary(path, phase, typ, custom),
		Description: description(path, phase, typ, custom),
		Tags:        tags(path, phase, typ),
		Parameters:  parseParametersFromPath(path),
	}
}

// operationID derives a unique, stable operation id from the route path and
// the action, eg. PATCH /api/sample/records/{id} -> "sample_records_patch".
// Deriving from the path instead of the model name keeps ids unique when
// same-named models exist in different packages or one model serves several
// routes; duplicate operation ids break OpenAPI client generators.
func operationID(path string, op apiaction.Phase) string {
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

// tags groups an operation under the first resource segment of its path,
// which matches the module structure of the backend (eg. sample, records,
// items). Path parameters never become tags.
func tags(path string, _ apiaction.Phase, typ reflect.Type) []string {
	segments := resourceSegments(strings.TrimSuffix(path, `/batch`))
	if len(segments) > 0 {
		return []string{segments[0]}
	}
	return []string{typ.Elem().Name()}
}

// operationDocInput assembles the apidoc.Operation describing one route
// operation for apidoc.DefaultSummary and apidoc.DefaultDescription.
// customTypes reports whether the operation declares its own request/response
// types instead of reusing the model (see apidoc.Operation.CustomTypes).
func operationDocInput(path string, phase apiaction.Phase, typ reflect.Type, customTypes bool) apidoc.Operation {
	elem := typ
	for elem.Kind() == reflect.Pointer || elem.Kind() == reflect.Slice {
		elem = elem.Elem()
	}
	return apidoc.Operation{
		Method:       phase.HTTPMethod(),
		Path:         path,
		Phase:        phase,
		CustomTypes:  customTypes,
		ModelName:    elem.Name(),
		ModelComment: openAPIStructComment(typ),
	}
}

// summary returns the operation summary: an explicitly registered
// apidoc.OperationDoc wins, otherwise apidoc.DefaultSummary builds it from the
// phase, path and model doc comment.
func summary(path string, phase apiaction.Phase, typ reflect.Type, customTypes bool) string {
	op := operationDocInput(path, phase, typ, customTypes)
	if doc, ok := apidoc.LookupOperation(op.Method, op.Path); ok && doc.Summary != "" {
		return doc.Summary
	}
	return apidoc.DefaultSummary(op)
}

// description returns the operation description: an explicitly registered
// apidoc.OperationDoc wins, otherwise apidoc.DefaultDescription builds it from
// the model doc comment.
func description(path string, phase apiaction.Phase, typ reflect.Type, customTypes bool) string {
	op := operationDocInput(path, phase, typ, customTypes)
	if doc, ok := apidoc.LookupOperation(op.Method, op.Path); ok && doc.Description != "" {
		return doc.Description
	}
	return apidoc.DefaultDescription(op)
}
