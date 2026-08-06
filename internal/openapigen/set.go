package openapigen

import (
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// Set registers the OpenAPI document entries for the given path and verbs.
// authRequired reports whether the route sits behind the authenticated route
// group; public routes are documented with an empty security requirement so
// they override the document-level security.
func Set[M types.Model, REQ types.Request, RSP types.Response](path string, authRequired bool, verb ...consts.HTTPVerb) {
	path = convertColonParamsToBraces(path)

	docMutex.Lock()
	pathItem := doc.Paths.Value(path)
	if pathItem == nil {
		pathItem = &openapi3.PathItem{}
		doc.Paths.Set(path, pathItem)
	}
	docMutex.Unlock()

	for _, verb := range buildVerbs(verb...) {
		var op *openapi3.Operation
		switch verb {
		case consts.Create:
			setCreate[M, REQ, RSP](path, pathItem)
			op = pathItem.Post
		case consts.Delete:
			setDelete[M, REQ, RSP](path, pathItem)
			op = pathItem.Delete
		case consts.Update:
			setUpdate[M, REQ, RSP](path, pathItem)
			op = pathItem.Put
		case consts.Patch:
			setPatch[M, REQ, RSP](path, pathItem)
			op = pathItem.Patch
		case consts.List:
			setList[M, REQ, RSP](path, pathItem)
			op = pathItem.Get
		case consts.Get:
			setGet[M, REQ, RSP](path, pathItem)
			op = pathItem.Get
		case consts.Import:
			setImport[M, REQ, RSP](path, pathItem)
			op = pathItem.Post
		case consts.Export:
			setExport[M, REQ, RSP](path, pathItem)
			op = pathItem.Get
		case consts.CreateMany:
			setCreateMany[M, REQ, RSP](path, pathItem)
			op = pathItem.Post
		case consts.DeleteMany:
			setDeleteMany[M, REQ, RSP](path, pathItem)
			op = pathItem.Delete
		case consts.UpdateMany:
			setUpdateMany[M, REQ, RSP](path, pathItem)
			op = pathItem.Put
		case consts.PatchMany:
			setPatchMany[M, REQ, RSP](path, pathItem)
			op = pathItem.Patch
		}
		if !authRequired {
			markPublic(op)
		}
	}

	docMutex.Lock()
	doc.Paths.Set(path, pathItem)
	docMutex.Unlock()
}

func buildVerbs(verbs ...consts.HTTPVerb) []consts.HTTPVerb {
	verbMap := make(map[consts.HTTPVerb]bool)
	for _, verb := range verbs {
		verbMap[verb] = true
	}

	vs := make([]consts.HTTPVerb, 0, len(verbMap))
	for verb := range verbMap {
		vs = append(vs, verb)
	}
	return vs
}

// convertColonParamsToBraces converts path parameters from :param to {param}.
func convertColonParamsToBraces(path string) string {
	if path == "" {
		return path
	}
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if strings.HasPrefix(seg, ":") && len(seg) > 1 {
			segments[i] = "{" + seg[1:] + "}"
		}
	}
	return strings.Join(segments, "/")
}
