package openapigen

import (
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// pending holds one closure per registered route, each capturing that route's
// generic type arguments. Registration only appends to it, because building the
// document means walking every model with reflection, and a process that never
// serves the document should not pay for that — which is most processes, and
// every test binary.
var (
	pendingMu sync.Mutex
	pending   []func()
)

// Set records one registered route for the OpenAPI document. It is the only
// entry point route registration uses.
func Set[M types.Model, REQ types.Request, RSP types.Response](path string, authRequired bool, verb consts.HTTPVerb) {
	pendingMu.Lock()
	defer pendingMu.Unlock()

	pending = append(pending, func() { set[M, REQ, RSP](path, authRequired, verb) })
}

// build turns the routes Set recorded into document entries, and refreshes the
// info block from live configuration. It runs on the first request for the
// document and holds the queue lock throughout, so a second concurrent request
// waits for a finished document rather than serving a half-built one. Later
// calls find an empty queue and cost nothing.
//
// Building on the request goroutine also keeps a panic inside the generator
// where the recovery middleware answers it, instead of on a bare goroutine that
// would take the process down.
func build() {
	pendingMu.Lock()
	defer pendingMu.Unlock()

	for _, register := range pending {
		register()
	}
	pending = nil

	docMutex.Lock()
	setDocInfo(doc)
	docMutex.Unlock()
}

// set registers the OpenAPI document entries for the given path and verbs.
// authRequired reports whether the route sits behind the authenticated route
// group; public routes are documented with an empty security requirement so
// they override the document-level security.
//
// Route registration goes through Set, which queues this call until the first
// request asks for the document; this package's own tests call set directly to
// exercise the generation.
func set[M types.Model, REQ types.Request, RSP types.Response](path string, authRequired bool, verb ...consts.HTTPVerb) {
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
		case consts.SSE:
			setSSE[M, REQ, RSP](path, pathItem)
			op = pathItem.Get
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
