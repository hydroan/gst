package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// registeredTestServices records the routes and phases registerTestService
// has registered a service under.
var registeredTestServices sync.Map

// registerTestService registers svc under phase and route unless an earlier
// run of the test already did: the registry refuses a second registration of
// a route and phase, which a test registering its service on every run would
// attempt under -count. A service a test asserts the state of is therefore
// shared across the runs, and the test resets that state first.
func registerTestService[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase, route string, svc types.Service[M, REQ, RSP]) {
	if _, registered := registeredTestServices.LoadOrStore(string(phase)+"|"+route, true); !registered {
		serviceregistry.Register[M, REQ, RSP](phase, route, svc)
	}
}

// handlerRouteModel is the model fixture the handler tests share:
// the route-dispatch and hook-tracing tests here, and the SSE test.
type handlerRouteModel struct {
	modelregistry.Base
}

// handlerRouteReq and handlerRouteRsp are shared by both action services
// below, mirroring how type aliases collapse distinct request and response
// declarations into one type.
type handlerRouteReq struct {
	Name string `json:"name"`
}

type handlerRouteRsp struct {
	Source string `json:"source"`
}

type handlerStartService struct {
	serviceregistry.Base[*handlerRouteModel, *handlerRouteReq, *handlerRouteRsp]
}

func (s *handlerStartService) Create(*types.ServiceContext, *handlerRouteReq) (*handlerRouteRsp, error) {
	return &handlerRouteRsp{Source: "start"}, nil
}

type handlerStopService struct {
	serviceregistry.Base[*handlerRouteModel, *handlerRouteReq, *handlerRouteRsp]
}

func (s *handlerStopService) Create(*types.ServiceContext, *handlerRouteReq) (*handlerRouteRsp, error) {
	return &handlerRouteRsp{Source: "stop"}, nil
}

// TestCreateHandlerDispatchesActionServiceByRoute guards the route-derived
// dispatch: two action services sharing one model/request/response type tuple
// must each receive the requests of their own route.
func TestCreateHandlerDispatchesActionServiceByRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	registerTestService[*handlerRouteModel, *handlerRouteReq, *handlerRouteRsp](consts.Create, "samples/:id/start", &handlerStartService{})
	registerTestService[*handlerRouteModel, *handlerRouteReq, *handlerRouteRsp](consts.Create, "samples/:id/stop", &handlerStopService{})

	engine := gin.New()
	engine.POST("/samples/:id/start", CreateHandler[*handlerRouteModel, *handlerRouteReq, *handlerRouteRsp](&types.ControllerConfig[*handlerRouteModel]{Route: "samples/:id/start"}))
	engine.POST("/samples/:id/stop", CreateHandler[*handlerRouteModel, *handlerRouteReq, *handlerRouteRsp](&types.ControllerConfig[*handlerRouteModel]{Route: "samples/:id/stop"}))

	for route, want := range map[string]string{
		"/samples/1/start": "start",
		"/samples/1/stop":  "stop",
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"name":"sample"}`))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, "route %s", route)
		require.Contains(t, recorder.Body.String(), want, "route %s must dispatch to its own service", route)
	}
}

// handlerListBeforeService overrides one list hook and leaves the other to the
// framework base.
type handlerListBeforeService struct {
	serviceregistry.Base[*handlerRouteModel, *handlerRouteModel, *handlerRouteModel]
}

func (*handlerListBeforeService) ListBefore(*types.ServiceContext, *[]*handlerRouteModel) error {
	return nil
}

// TestTraceServiceHookSpansOnlyOverriddenHooks verifies that a service hook
// gets a span of its own only when the service overrides it: the framework
// base's no-op still runs, but there is nothing to time, so it must not
// export a span.
func TestTraceServiceHookSpansOnlyOverriddenHooks(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	a := newAction[*handlerRouteModel, *handlerRouteModel, *handlerRouteModel]("samples", consts.List, consts.ListBefore, consts.ListAfter)

	t.Run("the_default_service_exports_no_hook_span", func(t *testing.T) {
		svc := serviceregistry.Resolve[*handlerRouteModel, *handlerRouteModel, *handlerRouteModel]("samples/unregistered")
		data := make([]*handlerRouteModel, 0)
		require.NoError(t, a.traceServiceHook(context.Background(), consts.ListBefore, svc, bareServiceContext, func(sc *types.ServiceContext) error {
			return svc.ListBefore(sc, &data)
		}))
		require.NotContains(t, oteltest.EndedNames(recorder), "service.HandlerRouteModel.ListBefore")
	})

	t.Run("an_overridden_hook_gets_a_span_and_its_no-op_partner_does_not", func(t *testing.T) {
		svc := &handlerListBeforeService{}
		data := make([]*handlerRouteModel, 0)
		require.NoError(t, a.traceServiceHook(context.Background(), consts.ListBefore, svc, bareServiceContext, func(sc *types.ServiceContext) error {
			return svc.ListBefore(sc, &data)
		}))
		require.NoError(t, a.traceServiceHook(context.Background(), consts.ListAfter, svc, bareServiceContext, func(sc *types.ServiceContext) error {
			return svc.ListAfter(sc, &data)
		}))
		names := oteltest.EndedNames(recorder)
		require.Contains(t, names, "service.HandlerRouteModel.ListBefore")
		require.NotContains(t, names, "service.HandlerRouteModel.ListAfter")
	})
}

// bareServiceContext builds the service context of a hook run outside any
// transport, carrying no request.
func bareServiceContext(ctx context.Context, phase consts.Phase) *types.ServiceContext {
	return types.NewServiceContext(nil, ctx, phase)
}
