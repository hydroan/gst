package types

import "io"

// Request and Response are the framework-facing types of one action's request
// and response payloads. They constrain the REQ and RSP type parameters of
// Service and Module; the concrete types are declared per action by the model
// layer.
type (
	Request  any
	Response any
)

// Service defines the controller-facing business operation contract for a model.
// Generated controllers call these methods for CRUD, batch CRUD, lifecycle hooks,
// import/export, filtering, and logging.
//
// Type Parameters:
//   - M: Model type that implements Model interface
//   - REQ: Request type for the current action or resource operation
//   - RSP: Response type for the current action or resource operation
//
// Custom actions should use action-specific REQ/RSP types instead of reusing
// types from other endpoints, even when the fields are identical.
//
// Nil-safety contract: when invoked by the generated controllers, ctx is
// never nil and req is never a nil pointer — the controller constructs a
// fresh *ServiceContext per call and instantiates REQ via reflection before
// binding, so implementations do not need defensive nil checks on ctx or req.
//
// Non-nil does not mean populated: List/Get never bind a request body, and
// Create/Update tolerate an empty body, so req may point to a zero-value
// struct. Validate required business fields instead of checking for nil.
//
// The contract only covers framework-invoked calls. Code that calls a
// service method directly (tests, jobs, or code bypassing the controller
// layer) must supply non-nil arguments itself.
type Service[M Model, REQ Request, RSP Response] interface {
	Create(*ServiceContext, REQ) (RSP, error)
	Delete(*ServiceContext, REQ) (RSP, error)
	Update(*ServiceContext, REQ) (RSP, error)
	Patch(*ServiceContext, REQ) (RSP, error)
	List(*ServiceContext, REQ) (RSP, error)
	Get(*ServiceContext, REQ) (RSP, error)

	CreateMany(*ServiceContext, REQ) (RSP, error)
	DeleteMany(*ServiceContext, REQ) (RSP, error)
	UpdateMany(*ServiceContext, REQ) (RSP, error)
	PatchMany(*ServiceContext, REQ) (RSP, error)

	CreateBefore(*ServiceContext, M) error
	CreateAfter(*ServiceContext, M) error
	DeleteBefore(*ServiceContext, M) error
	DeleteAfter(*ServiceContext, M) error
	UpdateBefore(*ServiceContext, M) error
	UpdateAfter(*ServiceContext, M) error
	PatchBefore(*ServiceContext, M) error
	PatchAfter(*ServiceContext, M) error
	ListBefore(*ServiceContext, *[]M) error
	ListAfter(*ServiceContext, *[]M) error
	GetBefore(*ServiceContext, M) error
	GetAfter(*ServiceContext, M) error

	CreateManyBefore(*ServiceContext, ...M) error
	CreateManyAfter(*ServiceContext, ...M) error
	DeleteManyBefore(*ServiceContext, ...M) error
	DeleteManyAfter(*ServiceContext, ...M) error
	UpdateManyBefore(*ServiceContext, ...M) error
	UpdateManyAfter(*ServiceContext, ...M) error
	PatchManyBefore(*ServiceContext, ...M) error
	PatchManyAfter(*ServiceContext, ...M) error

	Import(*ServiceContext, io.Reader) ([]M, error)
	Export(*ServiceContext, ...M) ([]byte, error)

	// SSE streams Server-Sent Events for the route: the implementation opens
	// the stream via ServiceContext.SSE and blocks until it is over. Query
	// parameters are read from ServiceContext.Query(). The action never binds
	// Payload or Result types, so the method carries no REQ or RSP.
	SSE(*ServiceContext) error

	// Filter lets a service rewrite the query condition before the
	// controller-side listing runs (List and Export). The model carries the
	// URL-decoded equality condition and the options carry the parsed operator
	// filters; the typical use is row-level data scoping: append typed filters
	// (e.g. Cols.GroupID.In(...)) to options.Filters or narrow the model
	// condition, then return both. Returning an error aborts the request — the
	// correct behavior when loading the caller's data scope fails. The
	// controller calls Filter once and shares the result between List and
	// Count, so both always see the same condition set.
	Filter(*ServiceContext, M, QueryOptions) (M, QueryOptions, error)

	Logger
}

// Module describes a registered API module: route metadata, auth exposure,
// resource parameter name, and the service implementation used by controllers.
//
// Type Parameters:
//   - M: Model type that implements Model interface
//   - REQ: Request type for API operations
//   - RSP: Response type for API operations
type Module[M Model, REQ Request, RSP Response] interface {
	// Service returns the service instance that handles business logic for this module.
	Service() Service[M, REQ, RSP]

	// Route returns the base API path for this module's endpoints.
	Route() string

	// Pub determines whether the API endpoints are public or require authentication.
	Pub() bool

	// Param returns the URL parameter name used for resource identification.
	Param() string
}

// ControllerConfig customizes how router.Register builds an internal handler for
// a route. It is the public configuration surface for controller behavior; the
// concrete controller handlers and their runtime state remain framework-owned.
type ControllerConfig[M Model] struct {
	// ParamName names the route parameter that carries the resource ID.
	ParamName string
	// Route is the raw route string the handler is registered under. Controller
	// factories derive the service registry key from it, so it must match the
	// route passed to the corresponding service.Register call. router.Register
	// fills it in automatically; an empty route resolves no service and the
	// handler falls back to the no-op default service.
	Route string
}
