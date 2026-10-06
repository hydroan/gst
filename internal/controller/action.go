package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// action is one action of a model on a route as the framework serves it,
// over HTTP and over gRPC alike: the type-derived values a handler or call
// needs on every request — reflection results, canonical span names, the
// service registry key — and, as its methods, the flows the action runs and
// the glue of each transport around them. Every XxxHandler builds one
// instance at route-registration time and every XxxCall one as the call
// function is built, each shared with all requests through the returned
// closure. All fields are read-only after construction, so concurrent
// requests can safely share one instance; request-scoped mutable state
// (model and request instances) is still created per request via newModel
// and newRequest.
type action[M types.Model, REQ types.Request, RSP types.Response] struct {
	typ        reflect.Type // struct type underlying M
	name       string       // struct name of M, recorded in span attributes and logs
	typesEqual bool         // whether M, REQ, and RSP are the same type
	phase      consts.Phase // the primary phase, the one the handler or call serves
	serviceKey string       // service registry key, resolved per request via serviceregistry.Resolve

	reqKind reflect.Kind // original kind of REQ before pointer dereferencing
	reqTyp  reflect.Type // struct type underlying REQ, used to build zero requests

	controllerSpan phaseSpan                  // controller span of the action's primary phase
	serviceSpans   map[consts.Phase]phaseSpan // service span names keyed by phase

	idColumn types.Column[string] // the id column of M, the reference the batch flows read records by
}

// idColumnName is the primary key column every framework model carries.
const idColumnName = "id"

// phaseSpan carries the precomputed span name and operation label of one phase.
type phaseSpan struct {
	name      string // canonical gst span name
	operation string // phase method name recorded in span attributes
}

// newAction builds the action of the primary phase on route, the one the
// handler or call is registered under, whose served path keys the service
// registry lookup together with the phase (see serviceregistry.Key); an
// empty route resolves no service, degrading to the no-op default service.
// hookPhases lists the additional service hook phases the action traces
// (for example the before/after phases of a CRUD operation), so their span
// names are precomputed as well. It panics, naming route, when REQ is an
// interface with methods or a pointer to one, a request type no request
// body decodes into.
func newAction[M types.Model, REQ types.Request, RSP types.Response](route string, phase consts.Phase, hookPhases ...consts.Phase) *action[M, REQ, RSP] {
	typ := reflect.TypeOf(*new(M)).Elem()
	name := typ.Name()

	reqTyp := reflect.TypeFor[REQ]()
	reqKind := reqTyp.Kind()
	for reqTyp.Kind() == reflect.Pointer {
		reqTyp = reqTyp.Elem()
	}
	// The request type is what a request body decodes into, and an interface
	// with methods admits no JSON value, pointed to or not: every request to
	// the route would fail to bind, so the declaration is refused as the route
	// registers.
	if reqTyp.Kind() == reflect.Interface && reqTyp.NumMethod() > 0 {
		panic(fmt.Sprintf("controller: route %q: request type %s is an interface with methods or a pointer to one, which no request body decodes into; declare a concrete type, or any", route, reflect.TypeFor[REQ]()))
	}

	serviceSpans := make(map[consts.Phase]phaseSpan, len(hookPhases)+1)
	serviceSpans[phase] = newPhaseSpan("service", name, phase)
	for _, hookPhase := range hookPhases {
		serviceSpans[hookPhase] = newPhaseSpan("service", name, hookPhase)
	}

	return &action[M, REQ, RSP]{
		typ:            typ,
		name:           name,
		typesEqual:     modelregistry.AreTypesEqual[M, REQ, RSP](),
		phase:          phase,
		serviceKey:     serviceregistry.Key(phase, route),
		reqKind:        reqKind,
		reqTyp:         reqTyp,
		controllerSpan: newPhaseSpan("controller", name, phase),
		serviceSpans:   serviceSpans,
		idColumn:       types.NewColumn[M, string](idColumnName),
	}
}

func newPhaseSpan(component, modelName string, phase consts.Phase) phaseSpan {
	return phaseSpan{
		name:      gstotel.FrameworkSpanName(component, modelName, phase.Name()),
		operation: phase.Name(),
	}
}

// serviceSpan returns the precomputed service span of the phase, falling back
// to on-the-fly construction for phases not declared when the action was
// built, so a missing declaration degrades to building the span per request
// instead of a wrong span name.
func (a *action[M, REQ, RSP]) serviceSpan(phase consts.Phase) phaseSpan {
	if span, ok := a.serviceSpans[phase]; ok {
		return span
	}
	return newPhaseSpan("service", a.name, phase)
}

// newModel returns a fresh model instance for one request. The instance is
// request-scoped mutable state and must never be cached on the action.
func (a *action[M, REQ, RSP]) newModel() M {
	return reflect.New(a.typ).Interface().(M) //nolint:errcheck
}

// newRequest returns the zero request value the delegation branch binds the
// request body into, preserving the construction rules for struct and pointer
// request types. Types that are neither struct nor pointer keep the plain zero
// value.
func (a *action[M, REQ, RSP]) newRequest() REQ {
	var req REQ
	switch a.reqKind {
	case reflect.Struct:
		req = reflect.New(a.reqTyp).Elem().Interface().(REQ) //nolint:errcheck
	case reflect.Pointer:
		req = reflect.New(a.reqTyp).Interface().(REQ) //nolint:errcheck
	}
	return req
}

// service resolves the phase service from the registry using the precomputed
// key. Resolution stays per request so services registered after route
// registration are still picked up.
func (a *action[M, REQ, RSP]) service() types.Service[M, REQ, RSP] {
	return serviceregistry.Resolve[M, REQ, RSP](a.serviceKey)
}

// startControllerSpan starts the span for the controller operation of the
// request c serves (see startSpan) and rebinds the request context so
// downstream layers nest under it.
func (a *action[M, REQ, RSP]) startControllerSpan(c *gin.Context) (context.Context, trace.Span) {
	spanCtx, span := a.startSpan(c.Request.Context(), c.Request.Method, c.FullPath())

	// Update request context with new span context
	c.Request = c.Request.WithContext(requestctx.WithMetadata(spanCtx, requestctx.FromGin(c)))

	return spanCtx, span
}

// startSpan starts the span for the controller operation under the request
// root span ctx carries, described by the method and route the action is
// served at, the same two for a request and for a call, as the registration
// described the call's action. The caller ends the span.
func (a *action[M, REQ, RSP]) startSpan(ctx context.Context, method, path string) (context.Context, trace.Span) {
	spanCtx, span := gstotel.StartSpan(gstotel.RequestRootContext(ctx), a.controllerSpan.name)

	// Attributes are built as typed values and submitted in one call, the same
	// shape the tracing middleware uses. Passing them as map[string]any instead
	// allocates the map, boxes every value into an interface, and then costs a
	// type switch to arrive back at these very attributes — per request, on the
	// sampled path.
	if gstotel.IsSpanRecording(span) {
		span.SetAttributes(
			attribute.String("component", "controller"),
			attribute.String("controller.operation", a.controllerSpan.operation),
			attribute.String("controller.model", a.name),
			attribute.String("controller.method", method),
			attribute.String("controller.path", path),
		)
	}

	return spanCtx, span
}

// traceServiceHook runs a service hook that returns only an error, on the
// service context newServiceContext builds for phase, and refuses, right
// after the hook returns, a hook that called a method of its context only an
// HTTP request can serve (see httpOnlyMethodCalled), so that the flow stops
// before what follows the hook, above all a record written after a before
// hook. A hook the service does not override is the framework base's no-op:
// it still runs, so the hook sequence stays the same for every service, but
// it has nothing worth timing and gets no span.
func (a *action[M, REQ, RSP]) traceServiceHook(parentCtx context.Context, phase consts.Phase, svc types.Service[M, REQ, RSP], newServiceContext serviceContextFunc, fn func(*types.ServiceContext) error) error {
	run := func(ctx context.Context) error {
		sc := newServiceContext(ctx, phase)
		if err := fn(sc); err != nil {
			return err
		}
		return httpOnlyMethodCalled(sc)
	}
	span := a.serviceSpan(phase)
	if !gstotel.IsEnabled() || !serviceregistry.OverridesHook(svc, span.operation) {
		return run(parentCtx)
	}
	_, err := traceServiceCall[struct{}](parentCtx, span, a.name, func(spanCtx context.Context) (struct{}, error) {
		return struct{}{}, run(spanCtx)
	})
	return err
}

// traceServiceOperation traces a delegated service operation returning RSP.
func (a *action[M, REQ, RSP]) traceServiceOperation(parentCtx context.Context, phase consts.Phase, fn func(context.Context) (RSP, error)) (RSP, error) {
	return traceServiceCall(parentCtx, a.serviceSpan(phase), a.name, fn)
}

// traceServiceCall runs fn inside a service span and records duration, success,
// and error attributes. It is the shared core of the traceService* methods; the
// duration and success attribute keys carry the "hook." prefix for every
// service call, the keys dashboards and queries select on.
func traceServiceCall[T any](parentCtx context.Context, span phaseSpan, modelName string, fn func(context.Context) (T, error)) (T, error) {
	spanCtx, s := gstotel.StartSpan(parentCtx, span.name)
	defer s.End()

	recording := gstotel.IsSpanRecording(s)
	if recording {
		s.SetAttributes(
			attribute.String("component", "service"),
			attribute.String("service.operation", span.operation),
			attribute.String("service.model", modelName),
		)
	}

	// Declare result variables for use in defer
	var err error
	var result T

	var startTime time.Time
	if recording {
		// Record start time and ensure duration + success recorded at the end
		startTime = time.Now()
	}
	defer func() {
		if recording {
			duration := time.Since(startTime)
			s.SetAttributes(
				attribute.Int64("hook.duration_ms", duration.Milliseconds()),
				attribute.Bool("hook.success", err == nil),
			)
			if err != nil {
				gstotel.RecordError(s, err)
			}
		}
	}()

	result, err = fn(spanCtx)
	return result, err
}
