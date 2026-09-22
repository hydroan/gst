package codegen

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/ds/tree/trie"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
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
func ResolveRoutes(models []*gen.ModelInfo, ignores []ggconfig.RouteRule) RouteIgnoreResult {
	buildHierarchicalEndpoints(models)
	propagateParentParams(models)
	return applyRouteIgnores(models, ignores)
}

// RouterTargetForAction returns the route the router registers action under,
// given the route its design declares it on, and the name of the path
// parameter that route ends in. An item action appends the design's parameter,
// :id when the design declares none: the Get action of route samples with
// Param("sample") registers samples/:sample, whose parameter is sample. A batch
// action appends batch, Import import and Export export: the CreateMany action
// of route samples registers samples/batch. An Exact action keeps the route it
// declares, parameter included.
func RouterTargetForAction(route string, design *dsl.Design, action *dsl.Action) (string, string) {
	if action == nil {
		return route, ""
	}

	if action.Exact {
		return route, routerPathParamName(route)
	}

	paramName := ""

	// If the phase is matched, the route appends the param, eg:
	// route "tenant" with param ":tenant" becomes "tenant/:tenant"
	// route "tenant" with param ":id" becomes "tenant/:id"
	switch action.Phase {
	case consts.PHASE_DELETE, consts.PHASE_UPDATE, consts.PHASE_PATCH, consts.PHASE_GET:
		param := ":id"
		if design != nil && len(design.Param) > 0 {
			param = design.Param
		}
		route = filepath.Join(route, param)
		paramName = routerPathParamName(route)
	case consts.PHASE_CREATE_MANY, consts.PHASE_DELETE_MANY, consts.PHASE_UPDATE_MANY, consts.PHASE_PATCH_MANY:
		route = filepath.Join(route, "batch")
	case consts.PHASE_IMPORT:
		route = filepath.Join(route, "import")
	case consts.PHASE_EXPORT:
		route = filepath.Join(route, "export")
	}

	return route, paramName
}

// routerPathParamName returns the name of the last parameter segment of
// route, written :name or {name}: id for iam/admin/users/:id/sessions, and ""
// for a route without one.
func routerPathParamName(route string) string {
	parts := strings.Split(route, "/")
	for _, part := range slices.Backward(parts) {
		trimmedPart := strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(trimmedPart, ":"):
			name := strings.TrimPrefix(trimmedPart, ":")
			if name != "" {
				return name
			}
		case strings.HasPrefix(trimmedPart, "{") && strings.HasSuffix(trimmedPart, "}"):
			name := strings.TrimSuffix(strings.TrimPrefix(trimmedPart, "{"), "}")
			if name != "" {
				return name
			}
		}
	}
	return ""
}

// buildHierarchicalEndpoints constructs complete hierarchical endpoint paths for all models.
// It maps directory structures to their corresponding endpoint names and builds full endpoint paths
// by replacing directory names with their custom endpoint names (if defined).
//
// For example:
//   - model/sample.go with Endpoint("samples") -> samples
//   - model/sample/item.go with Endpoint("items") -> samples/items
//   - model/sample/item/entry.go with Endpoint("entries") -> samples/items/entries
func buildHierarchicalEndpoints(allModels []*gen.ModelInfo) {
	// Create a map to store directory-to-endpoint mappings
	// This will store what endpoint name should be used for each directory
	dirEndpointMap := make(map[string]string)

	// First pass: build directory-to-endpoint mapping
	for _, m := range allModels {
		if m.Design == nil {
			continue
		}

		// Extract directory from model file path
		modelFilePath := strings.TrimPrefix(m.ModelFilePath, "model/")
		fileDir := filepath.Dir(modelFilePath)
		if fileDir == "." {
			fileDir = ""
		}

		// Get the filename without extension
		fileName := strings.TrimSuffix(filepath.Base(modelFilePath), ".go")

		// Determine the directory path that this model defines endpoint for
		// The rule is: model file defines endpoint for the directory path formed by modelDir + fileName
		var targetDir string
		if fileDir == "" {
			targetDir = fileName
		} else {
			targetDir = filepath.Join(fileDir, fileName)
		}

		// Store the endpoint mapping for the target directory
		if m.Design.Endpoint != "" {
			dirEndpointMap[targetDir] = m.Design.Endpoint
		}
	}

	// Second pass: build complete endpoints by replacing directory names with mapped endpoints
	for _, m := range allModels {
		if m.Design == nil {
			continue
		}

		// Extract directory from model file path
		modelFilePath := strings.TrimPrefix(m.ModelFilePath, "model/")
		fileDir := filepath.Dir(modelFilePath)
		if fileDir == "." {
			fileDir = ""
		}

		// Store the original endpoint from DSL
		originalEndpoint := m.Design.Endpoint

		if fileDir == "" {
			// Model is in root model directory, keep original endpoint
			continue
		}

		// Build the complete endpoint path by replacing directory names with mapped endpoints
		var endpointParts []string
		pathParts := strings.Split(fileDir, "/")

		// For each directory level, use mapped endpoint or directory name
		for i := range pathParts {
			currentPath := strings.Join(pathParts[:i+1], "/")
			if mappedEndpoint, exists := dirEndpointMap[currentPath]; exists {
				// Use the mapped endpoint for this directory
				endpointParts = append(endpointParts, mappedEndpoint)
			} else {
				// No mapping found, use directory name
				endpointParts = append(endpointParts, pathParts[i])
			}
		}

		// Add the current model's original endpoint
		endpointParts = append(endpointParts, originalEndpoint)

		// Join all parts to form the complete endpoint
		m.Design.Endpoint = strings.Join(endpointParts, "/")
	}
}

// propagateParentParams propagates the parameter of every parent resource into
// the endpoints of its descendants, so a nested resource is addressed inside the
// scope of the resources it belongs to. The endpoints are organized in a trie,
// which hands each of them its ancestors in one lookup.
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
func propagateParentParams(allModels []*gen.ModelInfo) {
	nodeFormater := trie.WithNodeFormatter[string, *gen.ModelInfo](func(v *gen.ModelInfo, depth int, hasValue bool) string {
		if !hasValue || v == nil {
			return "<nil>"
		}
		return fmt.Sprintf("%s (param: %s)", v.Design.Endpoint, v.Design.Param)
	})
	keyFormater := trie.WithKeyFormatter[string, *gen.ModelInfo](func(k string, v *gen.ModelInfo, depth int, hasValue bool) string {
		return k
	})

	// Create a trie tree to organize endpoints hierarchically
	// Key type is string, value type is *gen.ModelInfo
	tree, err := trie.New[string, *gen.ModelInfo](nodeFormater, keyFormater)
	if err != nil {
		panic(err)
	}

	// Build the trie tree
	for _, m := range allModels {
		// Split endpoint into segments for trie insertion
		// e.g., "samples/items/entries" -> ["samples", "items", "entries"]
		tree.Put(strings.Split(m.Design.Endpoint, "/"), m)
	}

	// Use trie's PathAncestors to collect parameters from all ancestor levels
	for _, model := range allModels {
		// Get all ancestors (including self) for this endpoint
		ancestors := tree.PathAncestors(strings.Split(model.Design.Endpoint, "/"))

		// Build the new endpoint path by inserting parameters from all ancestors
		newPathSegments := make([]string, 0)

		// Process each ancestor to build the hierarchical path with parameters
		// Note: ancestors[len(ancestors)-1] is the model itself, so we exclude it from parameter propagation
		for i, ancestor := range ancestors {
			// Add path segments from this ancestor level
			if i == 0 {
				// First ancestor: add all its path segments
				newPathSegments = append(newPathSegments, ancestor.Keys...)
			} else {
				// Subsequent ancestors: add only the new segments (difference from previous)
				prevAncestor := ancestors[i-1]
				if len(ancestor.Keys) > len(prevAncestor.Keys) {
					// Add the new segments
					newSegments := ancestor.Keys[len(prevAncestor.Keys):]
					newPathSegments = append(newPathSegments, newSegments...)
				}
			}

			// Add the parameter for this ancestor (if it has one)
			// But skip the last ancestor (which is the model itself) to avoid duplicate parameters
			if i < len(ancestors)-1 && ancestor.Value != nil && len(ancestor.Value.Design.Param) > 0 {
				param := ancestor.Value.Design.Param
				newPathSegments = append(newPathSegments, param)
			}
		}

		// Update the model's endpoint with the new path that includes all ancestor parameters
		if len(newPathSegments) > 0 {
			newEndpoint := strings.Join(newPathSegments, "/")
			model.Design.Endpoint = newEndpoint
		}
	}
}
