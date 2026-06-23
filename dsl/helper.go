package dsl

import (
	"maps"
	"slices"
	"strings"

	"github.com/hydroan/gst/types/consts"
)

var routePhaseOrder = []consts.Phase{
	consts.PHASE_CREATE,
	consts.PHASE_DELETE,
	consts.PHASE_UPDATE,
	consts.PHASE_PATCH,
	consts.PHASE_LIST,
	consts.PHASE_IMPORT,
	consts.PHASE_EXPORT,
	consts.PHASE_GET,
	consts.PHASE_CREATE_MANY,
	consts.PHASE_DELETE_MANY,
	consts.PHASE_UPDATE_MANY,
	consts.PHASE_PATCH_MANY,
}

// rangeAction iterates through all actions in a Design and calls the provided function
// for each enabled action. This is a helper function used by Design.Range().
//
// The function iterates through actions in a predefined order and only processes
// actions that are enabled (action.Enabled == true).
//
// Parameters:
//   - d: The Design containing actions to iterate over
//   - fn: Callback function that receives (endpoint, action) for each enabled action
//
// Iteration order:
//  1. Single record operations: Create, Delete, Update, Patch, List
//  2. Data transfer operations: Import, Export
//  3. Single record retrieval: Get
//  4. Batch operations: CreateMany, DeleteMany, UpdateMany, PatchMany
//
// For each enabled action, the callback receives:
//   - endpoint: The API endpoint path from the Design
//   - action: The Action configuration
//
// Example:
//
//	rangeAction(design, func(route string, a *Action,) {
//		fmt.Printf("%s %s payload=%s result=%s\n", action.Phase.MethodName(), route, a.Payload, a.Result)
//	})
func rangeAction(d *Design, fn func(string, *Action)) {
	if d == nil || fn == nil || !d.Enabled {
		return
	}

	if d.Create.Enabled {
		fn(d.Endpoint, d.Create)
	}
	if d.Delete.Enabled {
		fn(d.Endpoint, d.Delete)
	}
	if d.Update.Enabled {
		fn(d.Endpoint, d.Update)
	}
	if d.Patch.Enabled {
		fn(d.Endpoint, d.Patch)
	}
	if d.List.Enabled {
		fn(d.Endpoint, d.List)
	}
	if d.Import.Enabled {
		fn(d.Endpoint, d.Import)
	}
	if d.Export.Enabled {
		fn(d.Endpoint, d.Export)
	}
	if d.Get.Enabled {
		fn(d.Endpoint, d.Get)
	}
	if d.CreateMany.Enabled {
		fn(d.Endpoint, d.CreateMany)
	}
	if d.DeleteMany.Enabled {
		fn(d.Endpoint, d.DeleteMany)
	}
	if d.UpdateMany.Enabled {
		fn(d.Endpoint, d.UpdateMany)
	}
	if d.PatchMany.Enabled {
		fn(d.Endpoint, d.PatchMany)
	}

	// Sort route keys to ensure deterministic iteration order.
	for _, route := range slices.Sorted(maps.Keys(d.routes)) {
		emitRouteActions(route, d.routes[route], fn)
	}
}

func emitRouteActions(route string, actions []*Action, fn func(string, *Action)) {
	if len(actions) == 0 || fn == nil {
		return
	}
	for _, phase := range routePhaseOrder {
		for _, action := range actions {
			if action == nil || !action.Enabled || action.Phase != phase {
				continue
			}
			fn(route, action)
		}
	}
}

// is checks if the given name is a valid DSL keywords.
// It verifies against the predefined list of supported DSL keywords.
//
// Parameters:
//   - name: The keyword to check
//
// Returns:
//   - bool: true if the name is a valid DSL keyword, false otherwise
func is(name string) bool {
	return slices.Contains(methodList, name)
}

// trimQuote removes surrounding quotes from a string.
// It trims double quotes ("), single quotes ('), and backticks (`) from both ends.
//
// Parameters:
//   - str: The string to trim quotes from
//
// Returns:
//   - string: The string with surrounding quotes removed
//
// Example:
//
//	trimQuote(`"hello"`) returns "hello"
//	trimQuote("'world'") returns "world"
func trimQuote(str string) string {
	return strings.TrimFunc(str, func(r rune) bool {
		return r == '`' || r == '"' || r == '\''
	})
}
