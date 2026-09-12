package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// Request and Response are the framework-facing types of one action's request
// and response payloads. They constrain the REQ and RSP type parameters of
// Service and Module; the concrete types are declared per action by the model
// layer.
type (
	Request = itypes.Request

	Response = itypes.Response
)

// Service defines the controller-facing business operation contract for a model.
// Generated controllers call these methods for CRUD, batch CRUD, lifecycle hooks,
// import/export, filtering, and logging.
type Service[M Model, REQ Request, RSP Response] = itypes.Service[M, REQ, RSP]

// Module describes a registered API module: route metadata, auth exposure,
// resource parameter name, and the service implementation used by controllers.
type Module[M Model, REQ Request, RSP Response] = itypes.Module[M, REQ, RSP]

// ControllerConfig customizes how router.Register builds an internal handler for
// a route. It is the public configuration surface for controller behavior; the
// concrete controller handlers and their runtime state remain framework-owned.
type ControllerConfig[M Model] = itypes.ControllerConfig[M]
