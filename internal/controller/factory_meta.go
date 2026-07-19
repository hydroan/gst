package controller

import (
	"context"
	"reflect"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/serviceregistry"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.opentelemetry.io/otel/trace"
)

// factoryMeta caches the type-derived values a factory handler needs on every
// request: reflection results, canonical span names, and the service registry
// key. Every XxxFactory builds one instance at route-registration time and
// shares it with all requests through the returned closure. All fields are
// read-only after construction, so concurrent requests can safely share one
// instance; request-scoped mutable state (model and request instances) is
// still created per request via newModel and newRequest.
type factoryMeta[M types.Model, REQ types.Request, RSP types.Response] struct {
	typ        reflect.Type // struct type underlying M
	name       string       // struct name of M, recorded in span attributes and logs
	fullName   string       // fully qualified struct name of M, used as log object key
	typesEqual bool         // whether M, REQ, and RSP are the same type
	svcKey     string       // service registry key, resolved per request via serviceregistry.Resolve

	reqKind reflect.Kind // original kind of REQ before pointer dereferencing
	reqTyp  reflect.Type // struct type underlying REQ, used to build zero requests

	controllerSpan phaseSpan                  // controller span of the factory's primary phase
	serviceSpans   map[consts.Phase]phaseSpan // service span names keyed by phase
}

// phaseSpan carries the precomputed span name and operation label of one phase.
type phaseSpan struct {
	name      string // canonical gst span name
	operation string // phase method name recorded in span attributes
}

// newFactoryMeta builds the shared metadata for a factory handling the primary
// phase. hookPhases lists the additional service hook phases the handler
// traces (for example the before/after phases of a CRUD operation), so their
// span names are precomputed as well.
func newFactoryMeta[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase, hookPhases ...consts.Phase) *factoryMeta[M, REQ, RSP] {
	typ := reflect.TypeOf(*new(M)).Elem()
	name := typ.Name()

	reqTyp := reflect.TypeFor[REQ]()
	reqKind := reqTyp.Kind()
	for reqTyp.Kind() == reflect.Pointer {
		reqTyp = reqTyp.Elem()
	}

	serviceSpans := make(map[consts.Phase]phaseSpan, len(hookPhases)+1)
	serviceSpans[phase] = newPhaseSpan("service", name, phase)
	for _, hookPhase := range hookPhases {
		serviceSpans[hookPhase] = newPhaseSpan("service", name, hookPhase)
	}

	return &factoryMeta[M, REQ, RSP]{
		typ:            typ,
		name:           name,
		fullName:       typ.String(),
		typesEqual:     modelregistry.AreTypesEqual[M, REQ, RSP](),
		svcKey:         serviceregistry.KeyFor[M, REQ, RSP](phase),
		reqKind:        reqKind,
		reqTyp:         reqTyp,
		controllerSpan: newPhaseSpan("controller", name, phase),
		serviceSpans:   serviceSpans,
	}
}

func newPhaseSpan(component, modelName string, phase consts.Phase) phaseSpan {
	return phaseSpan{
		name:      gstotel.FrameworkSpanName(component, modelName, phase.MethodName()),
		operation: phase.MethodName(),
	}
}

// serviceSpan returns the precomputed service span of the phase, falling back
// to on-the-fly construction for phases not declared at meta construction so a
// missing declaration degrades to the old per-request cost instead of a wrong
// span name.
func (meta *factoryMeta[M, REQ, RSP]) serviceSpan(phase consts.Phase) phaseSpan {
	if span, ok := meta.serviceSpans[phase]; ok {
		return span
	}
	return newPhaseSpan("service", meta.name, phase)
}

// newModel returns a fresh model instance for one request. The instance is
// request-scoped mutable state and must never be cached on the meta.
func (meta *factoryMeta[M, REQ, RSP]) newModel() M {
	return reflect.New(meta.typ).Interface().(M) //nolint:errcheck
}

// newRequest returns the zero request value the delegation branch binds the
// request body into, preserving the construction rules for struct and pointer
// request types. Types that are neither struct nor pointer keep the plain zero
// value.
func (meta *factoryMeta[M, REQ, RSP]) newRequest() REQ {
	var req REQ
	switch meta.reqKind {
	case reflect.Struct:
		req = reflect.New(meta.reqTyp).Elem().Interface().(REQ) //nolint:errcheck
	case reflect.Pointer:
		req = reflect.New(meta.reqTyp).Interface().(REQ) //nolint:errcheck
	}
	return req
}

// service resolves the phase service from the registry using the precomputed
// key. Resolution stays per request so services registered after route
// registration are still picked up.
func (meta *factoryMeta[M, REQ, RSP]) service() types.Service[M, REQ, RSP] {
	return serviceregistry.Resolve[M, REQ, RSP](meta.svcKey)
}

// startControllerSpan starts the span for the controller operation and rebinds
// the request context so downstream layers nest under it.
func (meta *factoryMeta[M, REQ, RSP]) startControllerSpan(c *gin.Context) (context.Context, trace.Span) {
	parentCtx := gstotel.RequestRootContext(c.Request.Context())
	spanCtx, span := gstotel.StartSpan(parentCtx, meta.controllerSpan.name)

	// Update request context with new span context
	c.Request = c.Request.WithContext(requestctx.WithMetadata(spanCtx, requestctx.FromGin(c)))

	if gstotel.IsSpanRecording(span) {
		gstotel.AddSpanTags(span, map[string]any{
			"component":            "controller",
			"controller.operation": meta.controllerSpan.operation,
			"controller.model":     meta.name,
			"controller.method":    c.Request.Method,
			"controller.path":      c.FullPath(),
		})
	}

	return spanCtx, span
}

// traceServiceHook traces a service hook that returns only an error.
func (meta *factoryMeta[M, REQ, RSP]) traceServiceHook(parentCtx context.Context, phase consts.Phase, fn func(context.Context) error) error {
	_, err := traceServiceCall[struct{}](parentCtx, meta.serviceSpan(phase), meta.name, func(spanCtx context.Context) (struct{}, error) {
		return struct{}{}, fn(spanCtx)
	})
	return err
}

// traceServiceOperation traces a delegated service operation returning RSP.
func (meta *factoryMeta[M, REQ, RSP]) traceServiceOperation(parentCtx context.Context, phase consts.Phase, fn func(context.Context) (RSP, error)) (RSP, error) {
	return traceServiceCall(parentCtx, meta.serviceSpan(phase), meta.name, fn)
}

// traceServiceExport traces the service export operation.
func (meta *factoryMeta[M, REQ, RSP]) traceServiceExport(parentCtx context.Context, phase consts.Phase, fn func(context.Context) ([]byte, error)) ([]byte, error) {
	return traceServiceCall(parentCtx, meta.serviceSpan(phase), meta.name, fn)
}

// traceServiceImport traces the service import operation.
func (meta *factoryMeta[M, REQ, RSP]) traceServiceImport(parentCtx context.Context, phase consts.Phase, fn func(context.Context) ([]M, error)) ([]M, error) {
	return traceServiceCall(parentCtx, meta.serviceSpan(phase), meta.name, fn)
}

// traceServiceCall runs fn inside a service span and records duration, success,
// and error attributes. It is the shared core of the traceService* methods; the
// span attribute keys keep the historical "hook." prefix for every service call
// so existing dashboards and queries stay valid.
func traceServiceCall[T any](parentCtx context.Context, span phaseSpan, modelName string, fn func(context.Context) (T, error)) (T, error) {
	spanCtx, s := gstotel.StartSpan(parentCtx, span.name)
	defer s.End()

	recording := gstotel.IsSpanRecording(s)
	if recording {
		gstotel.AddSpanTags(s, map[string]any{
			"component":         "service",
			"service.operation": span.operation,
			"service.model":     modelName,
		})
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
			gstotel.AddSpanTags(s, map[string]any{
				"hook.duration_ms": duration.Milliseconds(),
				"hook.success":     err == nil,
			})
			if err != nil {
				gstotel.RecordError(s, err)
			}
		}
	}()

	result, err = fn(spanCtx)
	return result, err
}
