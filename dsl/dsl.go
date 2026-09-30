// Package dsl provides a Domain Specific Language (DSL) for defining REST API designs for Go models.
//
// The DSL allows developers to declaratively specify API configurations for their data models,
// including CRUD operations, endpoints, payload/result types, and various behavioral settings.
// It supports automatic code generation for services, controllers, and API routes based on
// the defined specifications.
//
// Basic Usage:
//
//	type User struct {
//		Name string
//		Email string
//		model.Base  // Embeds base model fields
//	}
//
//	func (User) Design() {
//		// Enable database migration (default: disabled)
//		Migrate()
//
//		// Set custom endpoint (default: pluralized snake_case model name)
//		Endpoint("users")
//
//		// Add path parameter for dynamic routing
//		Param("user")  // Creates routes like /api/users/:user
//
//		// Define alternative routes for different access patterns
//		Route("public/users", func() {
//			List(func() { Public() })
//			Get(func() { Public() })
//		})
//
//		// Configure Create operation
//		Create(func() {
//			Service() // Generate service code
//			// Omit Public() for authenticated APIs.
//			Payload[*CreateUserRequest]()
//			Result[*User]()
//		})
//
//		// Configure other operations...
//		Update(func() {})
//		Delete(func() {})
//		List(func() {})
//		Get(func() {})
//	}
//
// Naming a service:
//
// When multiple Route definitions share the same operation type (e.g., both use Create),
// give Service a name, which names the service file, its type and its rpc:
//
//	Route("/items/archive", func() {
//		Create(func() {
//			Service("archive")  // generates archive.go instead of create.go
//		})
//	})
//
// Supported Operations:
//   - Create, Update, Delete, Patch: Single record operations
//   - CreateMany, UpdateMany, DeleteMany, PatchMany: Batch operations
//   - List, Get: Read operations
//   - Import, Export: Data transfer operations, HTTP only
//   - SSE: Server-Sent Events streaming operations, HTTP only
//   - Stream: gRPC streaming operations, on a model declaring GRPC()
//
// Model Types:
//   - Models with model.Base: Full-featured models with database persistence
//   - Models with model.Empty: Lightweight models without database migration
//
// The keywords do nothing when called: gg gen reads a model's Design()
// as source and derives the routes, services and messages from it (see the
// internal dsl package of the framework), so a Design() runs no code.
// Each keyword forwards to its declaration in that package, which is what
// the parser knows; the documentation of the keywords is here, where the
// projects read it.
package dsl

import (
	"github.com/hydroan/gst/internal/dsl"
)

// Endpoint sets a custom endpoint path for the model's API routes.
// If not specified, defaults to the pluralized snake_case form of the model name,
// e.g. "sample_records" for a SampleRecord model.
// Leading slashes are automatically removed and forward slashes are replaced with hyphens.
// Example: Endpoint("users") for a User model, Endpoint("/iam/users") becomes "iam-users"
func Endpoint(path string) { dsl.Endpoint(path) }

// Param defines a path parameter for dynamic routing in RESTful APIs.
// It adds a URL parameter segment to the endpoint, enabling hierarchical resource access.
// The parameter is automatically propagated to child resources in nested structures,
// allowing parent resource parameters to be inherited by child endpoints.
//
// Parameter Format:
//   - Simple name: Param("user") creates ":user" parameter
//   - Bracketed format: Param("{user}") also creates ":user" parameter
//
// Route Generation Examples:
//   - Param("user") transforms /api/users to /api/users/:user
//   - Param("item") transforms /api/samples/items to /api/samples/items/:item
//   - Param("entry") transforms /api/samples/items/entries to /api/samples/items/entries/:entry
//
// Parameter Propagation:
// When using hierarchical models (sample -> item -> entry), parent parameters are
// automatically propagated to child resources:
//   - /api/samples/:sample/items/:item/entries/:entry
//   - Child resources inherit all parent path parameters
//
// The parameter creates RESTful nested resource patterns, enabling hierarchical API designs
// where child resources are scoped under parent resources through URL path parameters.
func Param(name string) { dsl.Param(name) }

// Route defines an alternative API route for the model beyond the default hierarchical route.
// This allows a resource to be accessible through multiple API endpoints, providing flexibility
// for different access patterns and use cases.
//
// The function accepts two parameters:
//   - path: The route path string (e.g., "items", "archive/items"). Leading slashes are automatically removed.
//   - config: A function that defines which operations are enabled for this route
//
// The function can be called multiple times within a Design() method to add multiple alternative routes.
// Each call adds a new route to the model's API endpoints without overriding existing ones.
//
// Route Format:
//   - Simple path: Route("items", func() {...}) creates /api/items
//   - Nested path: Route("archive/items", func() {...}) creates /api/archive/items
//   - Custom path: Route("admin/items", func() {...}) creates /api/admin/items
//   - Leading slash removed: Route("/archive/items", func() {...}) becomes "archive/items"
//
// Configuration Function:
// The second parameter is a function that defines which operations are available for this route.
// You can configure List, Get, Create, Update, Delete, Patch operations within this function:
//
//	Route("/archive/items", func() {
//	    List(func() {
//	        Service()
//	    })
//	    Get(func() {
//	        Service()
//	    })
//	})
//
// Route Generation:
// For a route path like "/archive/items" with Param("item"), the following routes are generated:
//   - /api/archive/items (for List operations)
//   - /api/archive/items/:item (for Get, Update, Delete, Patch operations)
//
// Use Exact() inside an action block when the action should use the route path
// exactly as declared instead of appending the default phase suffix.
//
// Usage Examples:
//   - Route("items", func() {...}) - Global item listing endpoint
//   - Route("archive/items", func() {...}) - Archive-scoped item endpoint
//   - Route("public/items", func() {...}) - Public item directory endpoint
//
// Common Use Cases:
//   - Global resource access: Access resources without parent constraints
//   - Alternative endpoints: Provide different API paths for the same resource
//   - Cross-cutting concerns: Admin, public, or system-level access patterns
//   - API versioning: Different route structures for API evolution
//
// Multiple Routes Example:
//
//	func (Item) Design() {
//	    Endpoint("items")
//	    Param("item")
//	    Route("items", func() {
//	        List(func() {})
//	        Get(func() {})
//	    })
//	    Route("archive/items", func() {
//	        List(func() { Service() })
//	        Get(func() { Service() })
//	    })
//	}
//
// This creates multiple API endpoints for the same model:
//   - /api/samples/:sample/items (default hierarchical route)
//   - /api/items and /api/items/:item (additional global route)
//   - /api/archive/items and /api/archive/items/:item (additional archive route)
func Route(path string, fn func()) { dsl.Route(path, fn) }

// Migrate marks the model as a database model that requires schema migration.
// When declared, the model's table structure will be created/updated in the database.
// Migration is disabled by default; declaring Migrate() enables it.
func Migrate() { dsl.Migrate() }

// GRPC serves the model over gRPC as well as HTTP: gg gen derives the model's
// .proto from its Go type and its Design() and generates the gRPC service
// beside the HTTP routes, both backed by the same service code. Declaring it
// is enabling it; a model without it is HTTP only. It can only be used at
// Design() top level, and needs at least one action gRPC can serve: Import,
// Export and SSE are HTTP only. A Stream action is served over gRPC alone
// and needs it.
func GRPC() { dsl.GRPC() }

// Service marks the current action as requiring custom service code, and
// names it.
//
// Service is an action-scoped marker and must be used inside an action block such
// as Create, List, or Get. Calling Service() tells gg gen to generate and
// register a service implementation for that action, named after the
// action: Create generates create.go declaring Creator, List generates
// list.go declaring Lister.
//
// Service("name") names the service instead: a bare name of letters, digits
// and underscores, starting with a letter, which names the generated file
// (name.go), the service type (its UpperCamelCase form) and, for a model
// declaring GRPC(), the rpc (the type name followed by the model name,
// MergeEntry for Service("merge") on Entry). Name the service when a model
// declares the same action on several routes, since two actions named after
// one phase would fight over one file, and for a custom action whose name
// says what it does; a Stream action always names its service, there being
// no default name for its rpc. Generated service log.Info messages use
// "{model}: {label}", label being the name with underscores replaced by
// spaces.
//
//	// Both routes of a model declared in model/sample/item.go declare
//	// Create; named, they generate service/sample/item/archive.go and
//	// service/sample/item/restore.go instead of one create.go:
//	Route("/items/archive", func() {
//	    Create(func() {
//	        Service("archive")
//	    })
//	})
//	Route("/items/restore", func() {
//	    Create(func() {
//	        Service("restore")
//	    })
//	})
//
// Omit Service when the framework default controller behavior is enough. This
// marker only controls service generation and registration for the current
// action; it does not change Payload, Result, Public, Exact, Flatten or route
// generation semantics.
func Service(name ...string) { dsl.Service(name...) }

// Flatten changes the service output layout for the current action.
//
// By default, gg treats each model file as its own service package:
//
//	model/authz/role.go + Service("role")
//	  -> service/authz/role/role.go
//	  -> package role
//
// Flatten removes the final model-file segment from the generated service path, so the
// action service is generated in the service package that mirrors the current model
// package:
//
//	model/authz/role.go + Service("role") + Flatten()
//	  -> service/authz/role.go
//	  -> package authz
//
// Flatten never accepts a directory name and cannot redirect output to another domain.
// The target directory and package are derived from the current model file's package.
// For example, model/authz/role.go cannot generate into service/mfa or service/authz2.
//
// Flatten only affects service generation. It does not change routes, model registration,
// payload/result types, or the service's name, which still controls only the generated
// file basename and service struct name. gg requires Flatten to be used with a named
// Service("name") in the same action.
//
// Flatten is only valid for model files under model/<package>/<file>.go. Root model files
// such as model/user.go cannot be flattened because service/ is reserved for generated
// registration code and should not contain business service files.
func Flatten() { dsl.Flatten() }

// Public marks the current action as publicly accessible.
//
// Public is an action-scoped marker and must be used inside an action block such
// as Create, List, or Get. Calling Public() registers the generated route on the
// public router, so authentication and authorization middleware are not required
// for that action.
//
// Omit Public() for authenticated APIs. The default is intentionally protected:
// actions are registered on the authenticated router unless they explicitly opt
// in to public access.
func Public() { dsl.Public() }

// Exact marks the current action route as exact.
//
// Exact is an action-scoped marker and must be used inside an action block such
// as Delete, Patch, or Get. Calling Exact() tells gg gen to register the route
// exactly as declared by Endpoint or Route, without appending the default phase
// suffix such as "/:id", "/batch", "/import", or "/export" for that action.
//
// Omit Exact() for normal CRUD-style route generation. Exact does not change
// Param, Public, Service, Payload, Result, Flatten, or any other DSL keyword;
// it only controls the current action's generated router path.
//
// Note that the built-in Delete, Update, and Patch controllers read the
// resource id from the route parameter only. An Exact route without an id
// segment must declare Payload/Result so the action is delegated to a custom
// service method instead of the built-in controller.
func Exact() { dsl.Exact() }

// Payload specifies the request payload type for the current action.
// The type parameter T defines the structure of incoming request data.
// Example: Payload[*CreateUserRequest]() or Payload[*User]()
//
// Payload must not be declared on List and Get actions: they handle HTTP GET
// requests, which carry no request body. Custom List/Get services declare
// Result only and read query parameters from ServiceContext.Query().
// Payload must not be declared on Import and Export actions either: they
// delegate to fixed service method signatures that never bind a request type.
func Payload[T any]() { dsl.Payload[T]() }

// Result specifies the response result type for the current action.
// The type parameter T defines the structure of outgoing response data.
// Example: Result[*User]() or Result[UserResponse]()
//
// Result must not be declared on Import and Export actions: they delegate to
// fixed service method signatures that never bind a response type.
func Result[T any]() { dsl.Result[T]() }

// Create defines the configuration for the create operation.
// The function parameter allows setting Service, Public, Payload, and Result.
// Declaring the action enables it.
// Example: Create(func() { Payload[*CreateUserRequest](); Result[*User]() })
func Create(fn func()) { dsl.Create(fn) }

// Delete defines the configuration for the delete operation.
// Typically used for soft or hard deletion of single records.
func Delete(fn func()) { dsl.Delete(fn) }

// Update defines the configuration for the update operation.
// Used for full record updates, replacing all fields.
func Update(fn func()) { dsl.Update(fn) }

// Patch defines the configuration for the patch operation.
// Used for partial record updates, modifying only specified fields.
func Patch(fn func()) { dsl.Patch(fn) }

// List defines the configuration for the list operation.
// Used for retrieving multiple records with optional filtering and pagination.
//
// List handles an HTTP GET request and must not declare Payload. Declaring
// Result delegates the action to a custom service method whose request type
// is generated as *model.Empty; filters are read from ServiceContext.Query().
func List(fn func()) { dsl.List(fn) }

// Get defines the configuration for the get operation.
// Used for retrieving a single record by identifier.
//
// Get handles an HTTP GET request and must not declare Payload. Declaring
// Result delegates the action to a custom service method whose request type
// is generated as *model.Empty; parameters are read from ServiceContext.Query()
// and ServiceContext.Param().
func Get(fn func()) { dsl.Get(fn) }

// CreateMany defines the configuration for batch create operations.
// Allows creating multiple records in a single request.
func CreateMany(fn func()) { dsl.CreateMany(fn) }

// DeleteMany defines the configuration for batch delete operations.
// Allows deleting multiple records in a single request.
func DeleteMany(fn func()) { dsl.DeleteMany(fn) }

// UpdateMany defines the configuration for batch update operations.
// Allows updating multiple records in a single request.
func UpdateMany(fn func()) { dsl.UpdateMany(fn) }

// PatchMany defines the configuration for batch patch operations.
// Allows partially updating multiple records in a single request.
func PatchMany(fn func()) { dsl.PatchMany(fn) }

// Import defines the configuration for data import operations.
// Used for bulk data ingestion from external sources.
//
// Import must not declare Payload or Result: the controller reads the
// uploaded multipart form file, delegates to the fixed service method
// Import(ctx, io.Reader) ([]M, error), and responds with a bare status code.
func Import(fn func()) { dsl.Import(fn) }

// Export defines the configuration for data export operations.
// Used for bulk data extraction to external formats.
//
// Export must not declare Payload or Result: the route handles an HTTP GET
// request whose filters come from query parameters, and the controller writes
// the bytes returned by the fixed service method
// Export(ctx, ...M) ([]byte, error) as a file attachment.
func Export(fn func()) { dsl.Export(fn) }

// SSE defines the configuration for a Server-Sent Events streaming operation.
// The route handles an HTTP GET request whose response is a long-lived
// text/event-stream; the registered route is automatically marked as
// streaming, which exempts it from request-scoped response treatment such as
// response body capture and request timeouts.
//
// SSE must declare Service(): there is no default streaming behavior, the
// fixed service method SSE(ctx) error opens the stream via ServiceContext.SSE
// and blocks until it is over. SSE must not declare Payload or Result — query
// parameters are read from ServiceContext.Query(), and the response is the
// event stream itself. An SSE action cannot share a route with List, as both
// register the GET route path itself.
func SSE(fn func()) { dsl.SSE(fn) }

// Stream defines a streaming operation, served over gRPC alone: one side of
// the call, or both, is a stream of messages rather than one message. The
// block declares the unary side with Payload or Result and the streaming
// side with StreamingPayload or StreamingResult, at least one side
// streaming: Payload with StreamingResult is a server stream,
// StreamingPayload with Result a client stream, and both streaming a
// bidirectional stream. A side declared neither way is *model.Empty.
//
// Stream must declare a named Service("name"), there being no built-in
// implementation and the name naming its rpc (see Service); it must not
// declare Exact(), having no HTTP route. It needs GRPC() on the model: HTTP carries
// no stream, so a Stream of a model without GRPC() would be served nowhere,
// and it is rejected. A Stream registers no HTTP route and appears in no
// OpenAPI document.
//
// Example, a server stream of the events of a feed on the route
// feeds/watch:
//
//	Route("feeds/watch", func() {
//	    Stream(func() {
//	        Service("watch")
//	        Payload[*FeedWatchReq]()
//	        StreamingResult[*FeedEvent]()
//	    })
//	})
func Stream(fn func()) { dsl.Stream(fn) }

// StreamingPayload declares the request side of a Stream action as a stream
// of T, one message per value the client sends. Example:
// StreamingPayload[*FeedEvent](). It can only be used inside a Stream block,
// which then cannot declare Payload as well.
func StreamingPayload[T any]() { dsl.StreamingPayload[T]() }

// StreamingResult declares the response side of a Stream action as a stream
// of T, one message per value the service sends. Example:
// StreamingResult[*FeedEvent](). It can only be used inside a Stream block,
// which then cannot declare Result as well.
func StreamingResult[T any]() { dsl.StreamingResult[T]() }
