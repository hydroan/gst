package apiaction

import (
	"slices"
	"strings"
)

// apiPathPrefix is the base every business API route is served under. It
// leaves this package only through APIPath, the one place it is added.
const apiPathPrefix = "/api"

// APIPath returns the path a route is served at, the route under the API
// prefix: /api/records/:id for records/:id, and for /records/:id,
// api/records/:id and /api/records/:id alike, since a leading or trailing
// slash and a prefix already written are dropped first; a blank route, or
// the prefix alone, names the root of the prefix, /api/. It is the one rule
// of the path of a route and the only place the prefix is added, and
// nothing takes it off again: gg gen writes every route by it, the router
// registers, the service registry keys and the modules register routes by
// it, and the gg commands list routes as it spells them. A route may thus
// be written with or without the prefix and names the same path either way.
func APIPath(route string) string {
	route = strings.Trim(route, "/")
	if prefix := strings.TrimPrefix(apiPathPrefix, "/"); route == prefix || strings.HasPrefix(route, prefix+"/") {
		route = strings.TrimPrefix(strings.TrimPrefix(route, prefix), "/")
	}
	return apiPathPrefix + "/" + route
}

// LastRouteParam returns the name of the last parameter segment of route,
// written :name: record for /api/records/:record, id for
// iam/admin/users/:id/sessions, and "" for records, a route naming none. It
// is the one rule of which parameter a route carries a record's id under: gg
// gen names the parameter of an item action's route and of an Exact route by
// it, and the gRPC call of an item action reads the id from it.
func LastRouteParam(route string) string {
	for _, part := range slices.Backward(strings.Split(route, "/")) {
		if name, ok := strings.CutPrefix(strings.TrimSpace(part), ":"); ok && name != "" {
			return name
		}
	}
	return ""
}

// ParamID is the parameter a route names one row's id under.
const ParamID = "id"

// batch is the route segment of the batch actions (see RouteSuffix).
const batch = "batch"

// RouteSuffix returns what the CRUD layout appends to a route for the
// action of phase, item being the parameter naming one row, as ":id": item
// for Delete, Update, Patch and Get, "batch" for the batch actions, "import"
// and "export" for Import and Export, and "" for the actions served on the
// route itself, Create, List and SSE. gg gen and module.Use register routes
// by it alike. For the route samples and the item :id, Get gives :id, so
// the route samples/:id, CreateMany batch, Export export and List nothing.
func RouteSuffix(phase Phase, item string) string {
	switch phase {
	case Delete, Update, Patch, Get:
		return item
	case CreateMany, DeleteMany, UpdateMany, PatchMany:
		return batch
	case Import:
		return import_
	case Export:
		return export
	}
	return ""
}
