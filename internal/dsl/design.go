// Package dsl reads the designs the models of a project declare with the
// keywords of the public dsl package: Parse turns a model file into the
// Design of each of its models, Validate reports the declarations the
// generator cannot honor, and Design and Action are what gg gen, gg check
// and the generators consume. The keywords stay in the public package, a
// project's model files being their one caller; the parser knows them by
// name (see methodList).
package dsl

import (
	"maps"
	"slices"
	"strings"

	"github.com/hydroan/gst/consts"
	"github.com/stoewer/go-strcase"
)

// PayloadEmpty is the Action.Payload value assigned to List and Get actions
// that declare Result. These actions handle HTTP GET requests without a
// request body, so code generation uses the non-persistent *model.Empty as
// the request type instead of the model type.
const PayloadEmpty = "*model.Empty"

// Design represents the complete API design configuration for a model.
// It contains global settings and individual action configurations.
// This struct is populated by parsing the model's Design() method.
type Design struct {
	// Endpoint specifies the URL path segment for this model's API routes.
	// Defaults to the pluralized snake_case form of the model name.
	// Used by the router to construct API endpoints.
	Endpoint string

	// Param contains the path parameter name for dynamic routing.
	// The parameter will be inserted as ":param" in the generated route paths.
	// Parameters are automatically propagated to child resources in nested structures,
	// allowing parent resource parameters to be inherited by child endpoints.
	//
	// Usage Examples:
	//   - Param("user") generates routes like /api/users/:user
	//   - Param("item") generates routes like /api/samples/items/:item
	//   - Param("entry") generates routes like /api/samples/items/entries/:entry
	//
	// Parameter Propagation:
	// In hierarchical models (sample -> item -> entry), parent parameters are
	// automatically propagated: /api/samples/:sample/items/:item/entries/:entry
	//
	// Default: "" (no parameter)
	Param string

	// routes contains alternative API routes for this model beyond the default hierarchical route.
	// Each route allows the resource to be accessible through alternative API endpoints,
	// providing flexibility for different access patterns and use cases.
	//
	// Map Structure:
	//   - Key: Route path string (e.g., "items", "archive/items", "public/items")
	//   - Value: Slice of Action configurations for operations enabled on this route
	//
	// Route Examples:
	//   - "items" creates /api/items and /api/items/:param (if Param is defined)
	//   - "archive/items" creates /api/archive/items and /api/archive/items/:param
	//   - "public/items" creates /api/public/items and /api/public/items/:param
	//
	// Action Configuration:
	// Each route can have different operations enabled. For example:
	//   - Route "items" might only enable List and Get operations
	//   - Route "admin/items" might enable all CRUD operations
	//   - Route "public/items" might only enable List operation
	//
	// Multiple routes can be defined by calling Route() multiple times in Design().
	// Each alternative route can have its own set of enabled operations and configurations.
	//
	// Usage in Design():
	//   Route("/archive/items", func() {
	//       List(func() {})
	//       Get(func() { Service() })
	//   })
	//
	// This populates routes["archive/items"] (the leading slash is removed) with
	// List and Get Action configurations.
	//
	// Default: nil (no alternative routes)
	routes map[string][]*Action

	// Migrate indicates whether database migration should be performed.
	// When true, the model's table structure will be created/updated.
	// Default: false
	Migrate bool

	// GRPC indicates whether the model is served over gRPC as well (see GRPC).
	// Default: false
	GRPC bool

	// IsEmpty indicates if the model contains a model.Empty field.
	// Models with model.Empty are lightweight and typically don't require migration.
	IsEmpty bool

	// Single record operations
	Create *Action // Create operation configuration
	Delete *Action // Delete operation configuration
	Update *Action // Update operation configuration (full replacement)
	Patch  *Action // Patch operation configuration (partial update)
	List   *Action // List operation configuration (retrieve multiple)
	Get    *Action // Get operation configuration (retrieve single)

	// Batch operations
	CreateMany *Action // Batch create operation configuration
	DeleteMany *Action // Batch delete operation configuration
	UpdateMany *Action // Batch update operation configuration
	PatchMany  *Action // Batch patch operation configuration

	// Data transfer operations
	Import *Action // Import operation configuration
	Export *Action // Export operation configuration

	// Streaming operations
	SSE    *Action // Server-Sent Events streaming operation configuration
	Stream *Action // gRPC streaming operation configuration (see Stream)
}

// Range iterates over the actions the Design declares and calls the provided function
// for each one, with the route the action is registered under. Nothing is called for a
// nil Design or a nil function.
//
// Parameters:
//   - fn: Callback function that receives (route, action) for each declared action
//
// The Design's own actions come first, under its endpoint, in a fixed order: Create,
// Delete, Update, Patch, List, Import, Export, SSE, Stream, Get, CreateMany,
// DeleteMany, UpdateMany, PatchMany. The actions declared with Route follow, route
// by route in sorted order, each route's actions in that same order.
//
// Example:
//
//	design.Range(func(route string, action *Action) {
//		fmt.Printf("Generating %s for %s\n", action.Phase.Name(), route)
//	})
func (d *Design) Range(fn func(route string, action *Action)) {
	if d == nil || fn == nil {
		return
	}

	for _, action := range d.ownActions() {
		if action != nil {
			fn(d.Endpoint, action)
		}
	}

	// Sort route keys to ensure deterministic iteration order.
	for _, route := range slices.Sorted(maps.Keys(d.routes)) {
		emitRouteActions(route, d.routes[route], fn)
	}
}

// ownActions returns the slots of the actions declared on the Design's own
// endpoint, in the order Range emits them; a slot is nil for an action the
// Design does not declare.
func (d *Design) ownActions() []*Action {
	return []*Action{d.Create, d.Delete, d.Update, d.Patch, d.List, d.Import, d.Export, d.SSE, d.Stream, d.Get, d.CreateMany, d.DeleteMany, d.UpdateMany, d.PatchMany}
}

// Drop removes the action from the Design, whichever route it is declared
// on: what a gst.yaml route ignore rule does to the action it matches, so
// that nothing is generated for it from then on. An action the Design does
// not hold is left alone. Drop is not for use inside the callback of Range.
func (d *Design) Drop(target *Action) {
	if d == nil || target == nil {
		return
	}
	for _, slot := range []**Action{&d.Create, &d.Delete, &d.Update, &d.Patch, &d.List, &d.Import, &d.Export, &d.SSE, &d.Stream, &d.Get, &d.CreateMany, &d.DeleteMany, &d.UpdateMany, &d.PatchMany} {
		if *slot == target {
			*slot = nil
			return
		}
	}
	for route, actions := range d.routes {
		if i := slices.Index(actions, target); i >= 0 {
			d.routes[route] = slices.Delete(actions, i, i+1)
			if len(d.routes[route]) == 0 {
				delete(d.routes, route)
			}
			return
		}
	}
}

// Action represents the configuration for a specific API operation.
// Each operation (Create, Update, Delete, etc.) has its own Action configuration.
type Action struct {
	// Service indicates whether custom service code should be generated and
	// registered for this action. It is true only when the action's DSL block
	// contains Service() or Service("name").
	// Default: false
	Service bool

	// Public indicates whether this action is registered on the public router.
	// It is true only when the action's DSL block contains Public().
	// Default: false, meaning the action requires authentication.
	Public bool

	// Exact indicates whether this action uses the route exactly as declared.
	// It is true only when the action's DSL block contains Exact().
	// Default: false, meaning the action uses the normal phase route pattern.
	Exact bool

	// Payload specifies the type name for the request payload.
	// This determines the structure of incoming request data.
	// Example: "CreateUserRequest", "*User", "User"
	// List and Get actions that declare Result carry PayloadEmpty here.
	Payload string

	// Result specifies the type name for the response result.
	// This determines the structure of outgoing response data.
	// Example: "*User", "UserResponse", "[]User"
	Result string

	// StreamingPayload marks the Payload of a Stream action as what each
	// message of the request stream carries, declared with
	// StreamingPayload; StreamingResult marks the Result of a Stream action
	// as what each message of the response stream carries, declared with
	// StreamingResult. Both are false for any other action.
	StreamingPayload bool
	StreamingResult  bool

	// ServiceName is the name Service("name") gives the action: the generated
	// service file is name.go, the service type its UpperCamelCase form, and
	// the rpc of a model declaring GRPC() that form followed by the model
	// name. Empty for Service(), which names them after the Phase.
	ServiceName string

	// Flatten indicates whether the generated service file should be written directly
	// into the service package that mirrors the current model package.
	// It only affects service output layout and requires Service("name").
	Flatten bool

	// The phase of the action
	// not part of DSL, just used to identify the current Action.
	Phase consts.Phase
}

// RoleName returns the struct name of the generated service: the
// UpperCamelCase form of the ServiceName, Archive for Service("archive") and
// ItemArchive for Service("item_archive"), or Phase.RoleName() for
// Service(), Creator for Create.
func (a *Action) RoleName() string {
	if a.ServiceName != "" {
		return strcase.UpperCamelCase(a.ServiceName)
	}
	return a.Phase.RoleName()
}

// ServiceFilename returns the name of the generated service file: the
// ServiceName in lower case plus .go, archive.go for Service("archive") and
// Service("Archive") alike, or the lower case Phase plus .go for Service(),
// create.go for Create.
func (a *Action) ServiceFilename() string {
	if a.ServiceName != "" {
		return strings.ToLower(a.ServiceName) + ".go"
	}
	return strings.ToLower(string(a.Phase)) + ".go"
}

var methodList = []string{
	"Endpoint",
	"Param",
	"Route",
	"Migrate",
	"GRPC",
	"Service",
	"Public",
	"Exact",
	"Payload",
	"Result",
	"Flatten",
	"StreamingPayload",
	"StreamingResult",

	consts.Create.Name(),
	consts.Delete.Name(),
	consts.Update.Name(),
	consts.Patch.Name(),
	consts.List.Name(),
	consts.Get.Name(),

	consts.CreateMany.Name(),
	consts.DeleteMany.Name(),
	consts.UpdateMany.Name(),
	consts.PatchMany.Name(),

	consts.Import.Name(),
	consts.Export.Name(),

	consts.SSE.Name(),
	consts.Stream.Name(),
}
