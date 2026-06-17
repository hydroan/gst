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
//		// Enable API generation (default: true)
//		Enabled(true)
//
//		// Set custom endpoint (default: lowercase model name)
//		Endpoint("users")
//
//		// Add path parameter for dynamic routing
//		Param("user")  // Creates routes like /api/users/:user
//
//		// Enable database migration (default: false)
//		Migrate(true)
//
//		// Define alternative routes for different access patterns
//		Route("public/users", func() {
//			List(func() { Public(true) })
//			Get(func() { Public(true) })
//		})
//
//		// Configure Create operation
//		Create(func() {
//			Service(true)   // Generate service code
//			Public(false)   // Internal API only
//			Payload[CreateUserRequest]()
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
// Custom Service Filenames:
//
// When multiple Route definitions share the same operation type (e.g., both use Create),
// use Filename to specify distinct service filenames:
//
//	Route("/attachment/upload", func() {
//		Create(func() {
//			Service(true)
//			Filename("upload")  // generates upload.go instead of create.go
//		})
//	})
//
// Supported Operations:
//   - Create, Update, Delete, Patch: Single record operations
//   - CreateMany, UpdateMany, DeleteMany, PatchMany: Batch operations
//   - List, Get: Read operations
//   - Import, Export: Data transfer operations
//
// Model Types:
//   - Models with model.Base: Full-featured models with database persistence
//   - Models with model.Empty: Lightweight models without database migration
package dsl

import (
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/types/consts"
	"github.com/stoewer/go-strcase"
)

// Enabled controls whether API generation is enabled.
// It has two usage scenarios:
//  1. When used in Design() method, it controls whether API generation is enabled for the entire model.
//     When set to false, no API code will be generated for this model.
//     Default: true
//  2. When used in action configuration functions (e.g., Create, Update, List, Get),
//     it controls whether the declared action should be generated.
//     Default: true for declared actions; actions that are not declared remain disabled.
func Enabled(bool) {}

// Endpoint sets a custom endpoint path for the model's API routes.
// If not specified, defaults to the lowercase version of the model name.
// Leading slashes are automatically removed and forward slashes are replaced with hyphens.
// Example: Endpoint("users") for a User model, Endpoint("/iam/users") becomes "iam-users"
func Endpoint(string) {}

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
//   - Param("app") transforms /api/namespaces/apps to /api/namespaces/apps/:app
//   - Param("env") transforms /api/namespaces/apps/envs to /api/namespaces/apps/envs/:env
//
// Parameter Propagation:
// When using hierarchical models (namespace -> app -> env), parent parameters are
// automatically propagated to child resources:
//   - /api/namespaces/:namespace/apps/:app/envs/:env
//   - Child resources inherit all parent path parameters
//
// Common Use Cases:
//   - namespace: Param("namespace") or Param("ns") for multi-tenant applications
//   - app: Param("app") for application-scoped resources
//   - env: Param("env") for environment-specific configurations
//
// The parameter creates RESTful nested resource patterns, enabling hierarchical API designs
// where child resources are scoped under parent resources through URL path parameters.
func Param(string) {}

// Route defines an alternative API route for the model beyond the default hierarchical route.
// This allows a resource to be accessible through multiple API endpoints, providing flexibility
// for different access patterns and use cases.
//
// The function accepts two parameters:
//   - path: The route path string (e.g., "apps", "config/apps"). Leading slashes are automatically removed.
//   - config: A function that defines which operations are enabled for this route
//
// The function can be called multiple times within a Design() method to add multiple alternative routes.
// Each call adds a new route to the model's API endpoints without overriding existing ones.
//
// Route Format:
//   - Simple path: Route("apps", func() {...}) creates /api/apps
//   - Nested path: Route("config/apps", func() {...}) creates /api/config/apps
//   - Custom path: Route("admin/applications", func() {...}) creates /api/admin/applications
//   - Leading slash removed: Route("/config/apps", func() {...}) becomes "config/apps"
//
// Configuration Function:
// The second parameter is a function that defines which operations are available for this route.
// You can configure List, Get, Create, Update, Delete, Patch operations within this function:
//
//	Route("/config/apps", func() {
//	    List(func() {
//	        Service(true)
//	    })
//	    Get(func() {
//	        Service(true)
//	    })
//	})
//
// Route Generation:
// For a route path like "/config/apps" with Param("app"), the following routes are generated:
//   - /api/config/apps (for List operations)
//   - /api/config/apps/:app (for Get, Update, Delete, Patch operations)
//
// Usage Examples:
//   - Route("apps", func() {...}) - Global app listing endpoint
//   - Route("config/apps", func() {...}) - Configuration-scoped app endpoint
//   - Route("public/apps", func() {...}) - Public app directory endpoint
//
// Common Use Cases:
//   - Global resource access: Access resources without namespace/parent constraints
//   - Alternative endpoints: Provide different API paths for the same resource
//   - Cross-cutting concerns: Admin, public, or system-level access patterns
//   - API versioning: Different route structures for API evolution
//
// Multiple Routes Example:
//
//	func (App) Design() {
//	    Endpoint("apps")
//	    Param("app")
//	    Route("apps", func() {
//	        List(func() {})
//	        Get(func() {})
//	    })
//	    Route("config/apps", func() {
//	        List(func() { Service(true) })
//	        Get(func() { Service(true) })
//	    })
//	}
//
// This creates multiple API endpoints for the same model:
//   - /api/namespaces/:ns/apps (default hierarchical route)
//   - /api/apps and /api/apps/:app (additional global route)
//   - /api/config/apps and /api/config/apps/:app (additional config route)
func Route(string, func()) {}

// Migrate controls whether database migration should be performed for this model.
// When true, the model's table structure will be created/updated in the database.
// Default: false
func Migrate(bool) {}

// Service controls whether service layer code should be generated for the current action.
// This affects the generation of business logic layer code.
// Default: false
func Service(bool) {}

// Filename specifies a custom filename (without extension) for the generated service file.
// When used inside an action configuration function (e.g., Create, Update), it overrides the
// default filename derived from the operation phase (e.g., "create", "list").
// Generated service log.Info messages use "{model}: {label}" where label is derived from this
// value: path base, underscores and hyphens replaced by spaces, whitespace collapsed.
//
// Background:
// By default, the generated service filename is derived from the operation phase name
// (e.g., Create generates "create.go", List generates "list.go"). When a model defines
// multiple Route with the same operation type, such as two routes both using Create,
// they share a single service file "create.go" because the filename is solely determined
// by the phase. This leads to the second route's generated code overwriting the first.
// Filename allows each action to specify a distinct output filename, ensuring that
// each route's service logic is generated into its own dedicated file.
//
// Default: "" (uses the lowercase phase name as filename, e.g., "create.go", "list.go")
//
// Example:
//
//	// Without Filename, both routes would generate service/shared/attachment/create.go,
//	// causing a conflict. With Filename, they produce separate files:
//	Route("/attachment/upload", func() {
//	    Create(func() {
//	        Service(true)
//	        Filename("upload")  // generates service/shared/attachment/upload.go
//	    })
//	})
//	Route("/attachment/parse", func() {
//	    Create(func() {
//	        Service(true)
//	        Filename("parse")   // generates service/shared/attachment/parse.go
//	    })
//	})
func Filename(string) {}

// Public controls whether the current action requires authentication/authorization.
// When false, the action will be processed by auth middleware if registered via middleware.RegisterAuth.
// When true, the action is publicly accessible without authentication.
// Default: false (requires authentication)
func Public(bool) {}

// Payload specifies the request payload type for the current action.
// The type parameter T defines the structure of incoming request data.
// Example: Payload[CreateUserRequest]() or Payload[*User]()
func Payload[T any]() {}

// Result specifies the response result type for the current action.
// The type parameter T defines the structure of outgoing response data.
// Example: Result[*User]() or Result[UserResponse]()
func Result[T any]() {}

// Create defines the configuration for the create operation.
// The function parameter allows setting Enabled, Service, Public, Payload, and Result.
// Declaring the action enables it by default.
// Example: Create(func() { Payload[CreateUserRequest](); Result[*User]() })
func Create(func()) {}

// Delete defines the configuration for the delete operation.
// Typically used for soft or hard deletion of single records.
func Delete(func()) {}

// Update defines the configuration for the update operation.
// Used for full record updates, replacing all fields.
func Update(func()) {}

// Patch defines the configuration for the patch operation.
// Used for partial record updates, modifying only specified fields.
func Patch(func()) {}

// List defines the configuration for the list operation.
// Used for retrieving multiple records with optional filtering and pagination.
func List(func()) {}

// Get defines the configuration for the get operation.
// Used for retrieving a single record by identifier.
func Get(func()) {}

// CreateMany defines the configuration for batch create operations.
// Allows creating multiple records in a single request.
func CreateMany(func()) {}

// DeleteMany defines the configuration for batch delete operations.
// Allows deleting multiple records in a single request.
func DeleteMany(func()) {}

// UpdateMany defines the configuration for batch update operations.
// Allows updating multiple records in a single request.
func UpdateMany(func()) {}

// PatchMany defines the configuration for batch patch operations.
// Allows partially updating multiple records in a single request.
func PatchMany(func()) {}

// Import defines the configuration for data import operations.
// Used for bulk data ingestion from external sources.
func Import(func()) {}

// Export defines the configuration for data export operations.
// Used for bulk data extraction to external formats.
func Export(func()) {}

// Design represents the complete API design configuration for a model.
// It contains global settings and individual action configurations.
// This struct is populated by parsing the model's Design() method.
type Design struct {
	// Enabled indicates whether API generation is enabled for this model.
	// Default: true
	Enabled bool

	// Endpoint specifies the URL path segment for this model's API routes.
	// Defaults to the lowercase version of the model name.
	// Used by the router to construct API endpoints.
	Endpoint string

	// Param contains the path parameter name for dynamic routing.
	// The parameter will be inserted as ":param" in the generated route paths.
	// Parameters are automatically propagated to child resources in nested structures,
	// allowing parent resource parameters to be inherited by child endpoints.
	//
	// Usage Examples:
	//   - Param("user") generates routes like /api/users/:user
	//   - Param("app") generates routes like /api/namespaces/apps/:app
	//   - Param("env") generates routes like /api/namespaces/apps/envs/:env
	//
	// Parameter Propagation:
	// In hierarchical models (namespace -> app -> env), parent parameters are
	// automatically propagated: /api/namespaces/:namespace/apps/:app/envs/:env
	//
	// Common Use Cases:
	//   - "namespace" or "ns": for multi-tenant applications
	//   - "app": for application-scoped resources
	//   - "env": for environment-specific configurations
	//
	// Default: "" (no parameter)
	Param string

	// routes contains alternative API routes for this model beyond the default hierarchical route.
	// Each route allows the resource to be accessible through alternative API endpoints,
	// providing flexibility for different access patterns and use cases.
	//
	// Map Structure:
	//   - Key: Route path string (e.g., "apps", "config/apps", "public/apps")
	//   - Value: Slice of Action configurations for operations enabled on this route
	//
	// Route Examples:
	//   - "apps" creates /api/apps and /api/apps/:param (if Param is defined)
	//   - "config/apps" creates /api/config/apps and /api/config/apps/:param
	//   - "public/apps" creates /api/public/apps and /api/public/apps/:param
	//
	// Action Configuration:
	// Each route can have different operations enabled. For example:
	//   - Route "apps" might only enable List and Get operations
	//   - Route "admin/apps" might enable all CRUD operations
	//   - Route "public/apps" might only enable List operation
	//
	// Multiple routes can be defined by calling Route() multiple times in Design().
	// Each alternative route can have its own set of enabled operations and configurations.
	//
	// Usage in Design():
	//   Route("/config/apps", func() {
	//       List(func() {})
	//       Get(func() { Service(true) })
	//   })
	//
	// This populates routes["/config/apps"] with List and Get Action configurations.
	//
	// Default: nil (no alternative routes)
	routes map[string][]*Action

	// Migrate indicates whether database migration should be performed.
	// When true, the model's table structure will be created/updated.
	// Default: false
	Migrate bool

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
}

// Range iterates over all enabled actions in the Design and calls the provided function
// for each one. The function receives the endpoint, action for each enabled action.
//
// Parameters:
//   - fn: Callback function that receives (endpoint, action) for each enabled action
//
// The iteration order is fixed: Create, Delete, Update, Patch, List, Import,
// Export, Get, CreateMany, DeleteMany, UpdateMany, PatchMany.
//
// Example:
//
//	design.Range(func(route string, action *Action) {
//		fmt.Printf("Generating %s for %s\n", action.Phase.MethodName(), route)
//	})
func (d *Design) Range(fn func(route string, action *Action)) { rangeAction(d, fn) }

// Action represents the configuration for a specific API operation.
// Each operation (Create, Update, Delete, etc.) has its own Action configuration.
type Action struct {
	// Enabled indicates whether this specific action should be generated.
	// Declared actions default to true; actions not declared in Design are disabled.
	Enabled bool

	// Service indicates whether service layer code should be generated for this action.
	// The service layer contains business logic and data access code.
	// Default: false
	Service bool

	// Public indicates whether this action requires authentication/authorization.
	// When false, the action will be processed by auth middleware if registered via middleware.RegisterAuth.
	// When true, the action is publicly accessible without authentication.
	// Default: false (requires authentication)
	Public bool

	// Payload specifies the type name for the request payload.
	// This determines the structure of incoming request data.
	// Example: "CreateUserRequest", "*User", "User"
	Payload string

	// Result specifies the type name for the response result.
	// This determines the structure of outgoing response data.
	// Example: "*User", "UserResponse", "[]User"
	Result string

	// Filename specifies a custom filename (without extension) for the generated service file.
	// When set, it overrides the default filename derived from the Phase.
	// For example, Filename="upload" generates "upload.go" instead of "create.go".
	// Default: "" (uses Phase-based filename)
	Filename string

	// The phase of the action
	// not part of DSL, just used to identify the current Action.
	Phase consts.Phase
}

// RoleName returns the struct name for the generated service file.
// If Filename is set, it extracts the base name (stripping any directory prefix
// and file extension) and converts it to UpperCamelCase.
// For example, Filename("upload") returns "Upload", Filename("a/b/user_upload.rs") returns "UserUpload".
// Otherwise, it falls back to Phase.RoleName() (e.g., "Creator", "Updater", "Deleter").
func (a *Action) RoleName() string {
	if len(a.Filename) > 0 {
		name := filepath.Base(a.Filename)
		name = strings.TrimSuffix(name, filepath.Ext(name))
		return strcase.UpperCamelCase(name)
	}
	return a.Phase.RoleName()
}

// ServiceFilename returns the filename for the generated service file.
// If Filename is set, it extracts the base name (stripping any directory prefix
// and file extension), converts it to lowercase, and appends ".go".
// For example, "a/b/c.rs" becomes "c.go", "Upload" becomes "upload.go".
// Otherwise, it falls back to the lowercase Phase name + ".go".
func (a *Action) ServiceFilename() string {
	if len(a.Filename) > 0 {
		name := filepath.Base(a.Filename)
		name = strings.TrimSuffix(name, filepath.Ext(name))
		return strings.ToLower(name) + ".go"
	}
	return strings.ToLower(string(a.Phase)) + ".go"
}

var methodList = []string{
	"Enabled",
	"Endpoint",
	"Param",
	"Route",
	"Migrate",
	"Payload",
	"Result",
	"Filename",

	consts.PHASE_CREATE.MethodName(),
	consts.PHASE_DELETE.MethodName(),
	consts.PHASE_UPDATE.MethodName(),
	consts.PHASE_PATCH.MethodName(),
	consts.PHASE_LIST.MethodName(),
	consts.PHASE_GET.MethodName(),

	consts.PHASE_CREATE_MANY.MethodName(),
	consts.PHASE_DELETE_MANY.MethodName(),
	consts.PHASE_UPDATE_MANY.MethodName(),
	consts.PHASE_PATCH_MANY.MethodName(),

	consts.PHASE_IMPORT.MethodName(),
	consts.PHASE_EXPORT.MethodName(),
}
