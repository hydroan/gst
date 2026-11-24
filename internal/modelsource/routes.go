package modelsource

import (
	"fmt"
	"path"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/ds/tree/trie"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
)

// ResolveRoutes gives the models, in place, the routes gg gen registers for
// them: every endpoint is nested under the endpoints of the model files its
// directories are named after (see buildHierarchicalEndpoints) and carries the
// parameter of each parent resource (see propagateParentParams), then every
// action whose route a gst.yaml gen.routes.ignore rule matches is disabled
// (see applyRouteIgnores). It returns how the ignore rules applied. The models
// are the ones FindModels finds under the model directory named relative to
// the project root, where gg runs, so every model file path starts with
// model/.
//
// For example, with model/sample.go declaring Endpoint("samples") and
// Param("sample") and model/sample/item.go declaring Endpoint("items"), the
// Item endpoint resolves to samples/:sample/items, and the rule
// GET /api/samples/:sample/items disables the Item List action.
func ResolveRoutes(models []*Model, ignores []ggconfig.RouteRule) RouteIgnoreResult {
	buildHierarchicalEndpoints(models)
	propagateParentParams(models)
	return applyRouteIgnores(models, ignores)
}

// RouteConflicts returns the conflicts among the routes the models
// register, once ResolveRoutes resolved them, each as an error naming both
// actions: two actions registering one path under one HTTP method, which
// the router refuses at startup, a Stream action among them under the GET
// that opens its connection, and two paths of one method naming a
// parameter differently at one position, which the router refuses as well
// (see parameterClash). The later action, in the order the models were
// found and their actions are declared, is held to the earlier one. For
// model/api/token.go and model/token.go, found in that order and both
// declaring a List action on Endpoint("tokens"), it returns
//
//	model/token.go: the List action of Token registers GET /api/tokens, as the List action of Token in model/api/token.go does; a path is served by one action
//
// and for a model declaring a Get action on Endpoint("records") and a List
// action on Route("records/:record/items")
//
//	model/record.go: the List action of Record registers GET /api/records/:record/items, naming the parameter :record where the Get action of Record in model/record.go, registering GET /api/records/:id, names :id; the router reads one parameter name at a position
func RouteConflicts(models []*Model) []error {
	type registration struct {
		file, model, action, path string
	}
	registered := make(map[string]registration)
	// The paths registered under each HTTP method, for the parameter names
	// of every later path to be held to.
	paths := make(map[string][]registration)
	var conflicts []error
	for _, m := range models {
		if m.Design == nil {
			continue
		}
		m.Design.Range(func(route string, act *dsl.Action) {
			path, _ := RouterTargetForAction(route, m.Design, act)
			method := act.Phase.HTTPMethod()
			key := method + " " + path
			current := registration{file: m.ModelFilePath, model: m.ModelName, action: act.Phase.Name(), path: path}
			if earlier, ok := registered[key]; ok {
				conflicts = append(conflicts, errors.Newf("%s: the %s action of %s registers %s, as the %s action of %s in %s does; a path is served by one action",
					current.file, current.action, current.model, key, earlier.action, earlier.model, earlier.file))
				return
			}
			registered[key] = current
			for _, earlier := range paths[method] {
				if name, earlierName, clash := parameterClash(path, earlier.path); clash {
					conflicts = append(conflicts, errors.Newf("%s: the %s action of %s registers %s %s, naming the parameter :%s where the %s action of %s in %s, registering %s %s, names :%s; the router reads one parameter name at a position",
						current.file, current.action, current.model, method, path, name, earlier.action, earlier.model, earlier.file, method, earlier.path, earlierName))
					break
				}
			}
			paths[method] = append(paths[method], current)
		})
	}
	return conflicts
}

// parameterClash reports whether the router would refuse path beside
// earlier, a path registered under the same method before it: the two name
// a parameter differently at one position after the same segments up to
// it, and the router's tree holds one parameter name at a position. It
// returns the two names, record and id for /api/records/:record/items
// beside /api/records/:id; a literal segment beside a parameter, batch
// beside :id, is served by the router and clashes with nothing.
func parameterClash(path, earlier string) (name, earlierName string, clash bool) {
	segments, earlierSegments := strings.Split(path, "/"), strings.Split(earlier, "/")
	for i := 0; i < len(segments) && i < len(earlierSegments); i++ {
		if segments[i] == earlierSegments[i] {
			continue
		}
		name, isParam := strings.CutPrefix(segments[i], ":")
		earlierName, earlierIsParam := strings.CutPrefix(earlierSegments[i], ":")
		return name, earlierName, isParam && earlierIsParam
	}
	return "", "", false
}

// ItemParam returns the path parameter the item actions (Get, Update, Patch,
// Delete) of a model append to its route: the parameter its design declares,
// :sample for Param("sample"), or :id when it declares none.
func ItemParam(design *dsl.Design) string {
	if design != nil && design.Param != "" {
		return design.Param
	}
	return ":" + apiaction.ParamID
}

// RouterTargetForAction returns the path the router registers action at,
// given the route its design declares it on, and the name of the path
// parameter that path ends in. The path is the route under the API prefix
// (see apiaction.APIPath), so every generated file spells a route the way it
// is served. An item action appends the parameter ItemParam returns: the Get
// action of route samples with Param("sample") registers /api/samples/:sample,
// whose parameter is sample. A batch action appends batch, Import import and
// Export export (see apiaction.RouteSuffix, the layout module.Use registers by
// too): the CreateMany action of route samples registers /api/samples/batch.
// An Exact action keeps the route it declares, parameter included:
// /api/iam/admin/users/:id/sessions, whose parameter is id, for
// iam/admin/users/:id/sessions.
func RouterTargetForAction(route string, design *dsl.Design, action *dsl.Action) (string, string) {
	route = apiaction.APIPath(route)
	if action == nil {
		return route, ""
	}

	if action.Exact {
		return route, apiaction.LastRouteParam(route)
	}

	// The phase decides what follows the route (see apiaction.RouteSuffix): the
	// item parameter for an action on one row, which the path then ends in
	// and names, route "tenant" with param ":tenant" becoming "tenant/:tenant"
	// and with param ":id" "tenant/:id", or the fixed segment of a batch,
	// import or export action.
	paramName := ""
	item := ItemParam(design)
	if suffix := apiaction.RouteSuffix(action.Phase, item); suffix != "" {
		route = path.Join(route, suffix)
		if suffix == item {
			paramName = apiaction.LastRouteParam(route)
		}
	}

	return route, paramName
}

// buildHierarchicalEndpoints constructs complete hierarchical endpoint paths
// for all models. It maps directory structures to their corresponding
// endpoint names and builds full endpoint paths by replacing directory names
// with their custom endpoint names (if defined).
//
// For example:
//   - model/sample.go with Endpoint("samples") -> samples
//   - model/sample/item.go with Endpoint("items") -> samples/items
//   - model/sample/item/entry.go with Endpoint("entries") -> samples/items/entries
func buildHierarchicalEndpoints(allModels []*Model) {
	// The endpoint each directory is addressed by, keyed by directory.
	dirEndpointMap := make(map[string]string)

	// First pass: build directory-to-endpoint mapping
	for _, m := range allModels {
		if m.Design == nil {
			continue
		}

		modelFilePath := m.pathInModelDir()
		fileDir := path.Dir(modelFilePath)
		if fileDir == "." {
			fileDir = ""
		}

		fileName := strings.TrimSuffix(path.Base(modelFilePath), ggconst.ExtensionGo)

		// A model file names the endpoint of the directory formed by its own
		// directory and its file name: model/sample/item.go names that of
		// sample/item.
		var targetDir string
		if fileDir == "" {
			targetDir = fileName
		} else {
			targetDir = path.Join(fileDir, fileName)
		}

		if m.Design.Endpoint != "" {
			dirEndpointMap[targetDir] = m.Design.Endpoint
		}
	}

	// Second pass: build the complete endpoints, each directory name replaced
	// by the endpoint mapped to it.
	for _, m := range allModels {
		if m.Design == nil {
			continue
		}

		fileDir := path.Dir(m.pathInModelDir())
		if fileDir == "." {
			fileDir = ""
		}

		originalEndpoint := m.Design.Endpoint

		if fileDir == "" {
			// Model is in root model directory, keep original endpoint
			continue
		}

		var endpointParts []string
		pathParts := strings.Split(fileDir, "/")

		// For each directory level, use mapped endpoint or directory name
		for i := range pathParts {
			currentPath := strings.Join(pathParts[:i+1], "/")
			if mappedEndpoint, exists := dirEndpointMap[currentPath]; exists {
				endpointParts = append(endpointParts, mappedEndpoint)
			} else {
				endpointParts = append(endpointParts, pathParts[i])
			}
		}

		endpointParts = append(endpointParts, originalEndpoint)

		m.Design.Endpoint = strings.Join(endpointParts, "/")
	}
}

// propagateParentParams propagates the parameter of every parent resource
// into the endpoints of its descendants, so a nested resource is addressed
// inside the scope of the resources it belongs to. The endpoints are organized
// in a trie, which hands each of them its ancestors in one lookup.
//
// For example, with model/sample.go declaring Endpoint("samples") and
// Param("sample"), model/sample/item.go declaring Endpoint("items") and
// Param("item"), and model/sample/item/entry.go declaring Endpoint("entries"),
// the endpoints
//
//	samples, samples/items, samples/items/entries
//
// become
//
//	samples, samples/:sample/items, samples/:sample/items/:item/entries
//
// so the last one registers routes such as GET and POST
// /api/samples/:sample/items/:item/entries.
func propagateParentParams(allModels []*Model) {
	nodeFormatter := trie.WithNodeFormatter[string, *Model](func(v *Model, depth int, hasValue bool) string {
		if !hasValue || v == nil {
			return "<nil>"
		}
		return fmt.Sprintf("%s (param: %s)", v.Design.Endpoint, v.Design.Param)
	})
	keyFormatter := trie.WithKeyFormatter[string, *Model](func(k string, v *Model, depth int, hasValue bool) string {
		return k
	})

	tree, err := trie.New[string, *Model](nodeFormatter, keyFormatter)
	if err != nil {
		panic(err)
	}

	for _, m := range allModels {
		// Split endpoint into segments for trie insertion
		// e.g., "samples/items/entries" -> ["samples", "items", "entries"]
		tree.Put(strings.Split(m.Design.Endpoint, "/"), m)
	}

	// Use trie's PathAncestors to collect parameters from all ancestor levels
	for _, model := range allModels {
		// Get all ancestors (including self) for this endpoint
		ancestors := tree.PathAncestors(strings.Split(model.Design.Endpoint, "/"))

		newPathSegments := make([]string, 0)

		// The last ancestor is the model itself, which adds no parameter of
		// its own to its endpoint.
		for i, ancestor := range ancestors {
			if i == 0 {
				// First ancestor: add all its path segments
				newPathSegments = append(newPathSegments, ancestor.Keys...)
			} else {
				// Subsequent ancestors: add only the segments the previous one
				// does not have.
				prevAncestor := ancestors[i-1]
				if len(ancestor.Keys) > len(prevAncestor.Keys) {
					newSegments := ancestor.Keys[len(prevAncestor.Keys):]
					newPathSegments = append(newPathSegments, newSegments...)
				}
			}

			// The parameter of this ancestor, if it has one.
			if i < len(ancestors)-1 && ancestor.Value != nil && len(ancestor.Value.Design.Param) > 0 {
				param := ancestor.Value.Design.Param
				newPathSegments = append(newPathSegments, param)
			}
		}

		if len(newPathSegments) > 0 {
			newEndpoint := strings.Join(newPathSegments, "/")
			model.Design.Endpoint = newEndpoint
		}
	}
}
