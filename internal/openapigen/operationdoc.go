package openapigen

import (
	"reflect"
	"strings"

	"github.com/hydroan/gst/apidoc"
	"github.com/hydroan/gst/types/consts"
)

// operationID derives a unique, stable operation id from the route path and
// the action, eg. PATCH /api/sample/records/{id} -> "sample_records_patch".
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

// tags groups an operation under the first resource segment of its path,
// which matches the module structure of the backend (eg. sample, records,
// items). Path parameters never become tags.
func tags(path string, _ consts.HTTPVerb, typ reflect.Type) []string {
	segments := resourceSegments(strings.TrimSuffix(path, `/batch`))
	if len(segments) > 0 {
		return []string{segments[0]}
	}
	return []string{typ.Elem().Name()}
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
