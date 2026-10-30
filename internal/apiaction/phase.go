package apiaction

import (
	"net/http"
	"strings"

	"github.com/stoewer/go-strcase"
)

// Phase is a phase of an action: the action itself, Create and its kind,
// Import, Export, SSE and Stream, and, for the CRUD and batch actions, the
// hooks run before and after it, CreateBefore and CreateAfter. The value is
// the snake case name the service registry keys by and the logs carry; the
// name of a phase, the identifier its constant is declared with, is the DSL
// keyword declaring the action and the service method serving it.
type Phase string

// The phases of an action. Generated code refers to a phase by its name
// (see Phase.Name), service.Create for Create.
const (
	Create Phase = create
	Delete Phase = delete_
	Update Phase = update
	Patch  Phase = patch
	List   Phase = list
	Get    Phase = get

	CreateMany Phase = create_many
	DeleteMany Phase = delete_many
	UpdateMany Phase = update_many
	PatchMany  Phase = patch_many

	CreateBefore Phase = create_before
	CreateAfter  Phase = create_after
	DeleteBefore Phase = delete_before
	DeleteAfter  Phase = delete_after
	UpdateBefore Phase = update_before
	UpdateAfter  Phase = update_after
	PatchBefore  Phase = patch_before
	PatchAfter   Phase = patch_after
	ListBefore   Phase = list_before
	ListAfter    Phase = list_after
	GetBefore    Phase = get_before
	GetAfter     Phase = get_after

	CreateManyBefore Phase = create_many_before
	CreateManyAfter  Phase = create_many_after
	DeleteManyBefore Phase = delete_many_before
	DeleteManyAfter  Phase = delete_many_after
	UpdateManyBefore Phase = update_many_before
	UpdateManyAfter  Phase = update_many_after
	PatchManyBefore  Phase = patch_many_before
	PatchManyAfter   Phase = patch_many_after

	Import Phase = import_
	Export Phase = export

	SSE Phase = sse

	Stream Phase = stream
)

// The values of the phases, each the snake case name of its phase.
const (
	create  = "create"
	delete_ = "delete"
	update  = "update"
	patch   = "patch"
	list    = "list"
	get     = "get"

	create_many = "create_many"
	delete_many = "delete_many"
	update_many = "update_many"
	patch_many  = "patch_many"

	create_before = "create_before"
	create_after  = "create_after"
	delete_before = "delete_before"
	delete_after  = "delete_after"
	update_before = "update_before"
	update_after  = "update_after"
	patch_before  = "patch_before"
	patch_after   = "patch_after"
	list_before   = "list_before"
	list_after    = "list_after"
	get_before    = "get_before"
	get_after     = "get_after"

	create_many_before = "create_many_before"
	create_many_after  = "create_many_after"
	delete_many_before = "delete_many_before"
	delete_many_after  = "delete_many_after"
	update_many_before = "update_many_before"
	update_many_after  = "update_many_after"
	patch_many_before  = "patch_many_before"
	patch_many_after   = "patch_many_after"

	import_ = "import"
	export  = "export"

	sse = "sse"

	stream = "stream"
)

// Name returns the name of the phase, the identifier its constant is
// declared with, which is the DSL keyword declaring the action and the
// service method serving it: the value in UpperCamelCase, SSE kept as the
// initialism. Generated code refers to a phase by it. Examples:
//
//	Create       -> "Create"
//	CreateBefore -> "CreateBefore"
//	UpdateMany   -> "UpdateMany"
//	SSE          -> "SSE"
func (p Phase) Name() string {
	if p == SSE {
		return "SSE"
	}
	return strcase.UpperCamelCase(string(p))
}

// Filename returns the name of the service file of the phase's action, the
// action's name with the .go suffix.
// Example:
//
//	Create       -> "create.go"
//	CreateBefore -> "create.go"
//	UpdateMany   -> "update_many.go"
func (p Phase) Filename() string {
	return string(p.action()) + ".go"
}

// action returns the phase of the action p belongs to: p itself for an
// action phase, the action of a hook phase, Create for CreateBefore and
// CreateMany for CreateManyAfter.
func (p Phase) action() Phase {
	s := strings.TrimSuffix(string(p), "_before")
	return Phase(strings.TrimSuffix(s, "_after"))
}

// hook reports whether p is a hook phase, one run before or after its
// action.
func (p Phase) hook() bool {
	return p.action() != p
}

// RoleName returns the associated role name for the Phase in human-readable
// form. It maps CRUD and ManyCRUD phases (including "_before" and "_after") to
// specific role names. Hook phases map to the role that hosts them: the
// list_before and list_after hooks are generated as methods on the Lister, so
// both map to "Lister". Unrecognized phases return an empty string.
//
// Examples:
//
//	Create          -> "Creator"
//	CreateBefore    -> "Creator"
//	UpdateMany      -> "ManyUpdater"
//	UpdateManyAfter -> "ManyUpdater"
func (p Phase) RoleName() string {
	s := string(p.action())
	isMany := strings.HasSuffix(s, "_many")
	s = strings.TrimSuffix(s, "_many")

	var role string
	switch s {
	case create:
		role = "Creator"
	case update:
		role = "Updater"
	case delete_:
		role = "Deleter"
	case patch:
		role = "Patcher"
	case list:
		role = "Lister"
	case get:
		role = "Getter"
	case import_:
		role = "Importer"
	case export:
		role = "Exporter"
	case sse:
		role = "Streamer"
	default:
		return ""
	}

	if isMany {
		role = "Many" + role
	}
	return role
}

// Before returns the corresponding "_before" Phase for CRUD or ManyCRUD
// operations. If the current Phase already contains "_before" or "_after", or
// is not CRUD/MANYCRUD, it returns itself without modification.
func (p Phase) Before() Phase {
	if p.hook() {
		return p
	}
	s := string(p)

	// Only handle standard CRUD actions
	parts := strings.Split(s, "_")
	if len(parts) == 0 {
		return p
	}

	switch parts[0] {
	case create, update, delete_, patch, list, get:
		// Keep "_many" before "_before"
		if strings.Contains(s, "_many") {
			return Phase(strings.TrimSuffix(s, "_many") + "_many_before")
		}
		return Phase(s + "_before")
	default:
		return p
	}
}

// After returns the corresponding "_after" Phase for CRUD or ManyCRUD
// operations. If the current Phase already contains "_before" or "_after", or
// is not CRUD/MANYCRUD, it returns itself without modification.
func (p Phase) After() Phase {
	if p.hook() {
		return p
	}
	s := string(p)

	// Only handle standard CRUD actions
	parts := strings.Split(s, "_")
	if len(parts) == 0 {
		return p
	}

	switch parts[0] {
	case create, update, delete_, patch, list, get:
		// Keep "_many" before "_after"
		if strings.Contains(s, "_many") {
			return Phase(strings.TrimSuffix(s, "_many") + "_many_after")
		}
		return Phase(s + "_after")
	default:
		return p
	}
}

// HTTPMethod returns the HTTP method of the route the phase is registered
// under: Create, CreateMany and Import map to "POST", Delete and DeleteMany
// to "DELETE", Update and UpdateMany to "PUT", Patch and PatchMany to
// "PATCH", and List, Get, Export and SSE to "GET". A hook phase, run inside
// its action rather than under a route of its own, Stream, served over gRPC
// alone, and any other phase map to "", which is how the router tells the
// phases it can serve. It is the one table of methods: the framework router
// registers routes by it, and gg reads route methods from it.
func (p Phase) HTTPMethod() string {
	switch p {
	case Create, CreateMany, Import:
		return http.MethodPost
	case Delete, DeleteMany:
		return http.MethodDelete
	case Update, UpdateMany:
		return http.MethodPut
	case Patch, PatchMany:
		return http.MethodPatch
	case List, Get, Export, SSE:
		return http.MethodGet
	}
	return ""
}
