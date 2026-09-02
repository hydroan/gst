package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// factoryRouteModel is the model fixture of the route-dispatch tests.
type factoryRouteModel struct {
	modelregistry.Base
}

// factoryRouteReq and factoryRouteRsp are shared by both action services
// below, mirroring how type aliases collapse distinct request and response
// declarations into one type.
type factoryRouteReq struct {
	Name string `json:"name"`
}

type factoryRouteRsp struct {
	Source string `json:"source"`
}

type factoryStartService struct {
	serviceregistry.Base[*factoryRouteModel, *factoryRouteReq, *factoryRouteRsp]
}

func (s *factoryStartService) Create(*types.ServiceContext, *factoryRouteReq) (*factoryRouteRsp, error) {
	return &factoryRouteRsp{Source: "start"}, nil
}

type factoryStopService struct {
	serviceregistry.Base[*factoryRouteModel, *factoryRouteReq, *factoryRouteRsp]
}

func (s *factoryStopService) Create(*types.ServiceContext, *factoryRouteReq) (*factoryRouteRsp, error) {
	return &factoryRouteRsp{Source: "stop"}, nil
}

// TestCreateFactoryDispatchesActionServiceByRoute guards the route-derived
// dispatch: two action services sharing one model/request/response type tuple
// must each receive the requests of their own route.
func TestCreateFactoryDispatchesActionServiceByRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger.Controller = zap.New("")

	serviceregistry.Register[*factoryRouteModel, *factoryRouteReq, *factoryRouteRsp](consts.PHASE_CREATE, "samples/:id/start", &factoryStartService{})
	serviceregistry.Register[*factoryRouteModel, *factoryRouteReq, *factoryRouteRsp](consts.PHASE_CREATE, "samples/:id/stop", &factoryStopService{})

	engine := gin.New()
	engine.POST("/samples/:id/start", CreateFactory[*factoryRouteModel, *factoryRouteReq, *factoryRouteRsp](&types.ControllerConfig[*factoryRouteModel]{Route: "samples/:id/start"}))
	engine.POST("/samples/:id/stop", CreateFactory[*factoryRouteModel, *factoryRouteReq, *factoryRouteRsp](&types.ControllerConfig[*factoryRouteModel]{Route: "samples/:id/stop"}))

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

// factoryListBeforeService overrides one list hook and leaves the other to the
// framework base.
type factoryListBeforeService struct {
	serviceregistry.Base[*factoryRouteModel, *factoryRouteModel, *factoryRouteModel]
}

func (*factoryListBeforeService) ListBefore(*types.ServiceContext, *[]*factoryRouteModel) error {
	return nil
}

// TestTraceServiceHookSpansOnlyOverriddenHooks verifies that a service hook
// gets a span of its own only when the service overrides it: the framework
// base's no-op still runs, but there is nothing to time, so it must not
// export a span.
func TestTraceServiceHookSpansOnlyOverriddenHooks(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	meta := newFactoryMeta[*factoryRouteModel, *factoryRouteModel, *factoryRouteModel]("samples", consts.PHASE_LIST, consts.PHASE_LIST_BEFORE, consts.PHASE_LIST_AFTER)

	t.Run("the default service exports no hook span", func(t *testing.T) {
		svc := serviceregistry.Resolve[*factoryRouteModel, *factoryRouteModel, *factoryRouteModel]("samples/unregistered")
		data := make([]*factoryRouteModel, 0)
		require.NoError(t, meta.traceServiceHook(context.Background(), consts.PHASE_LIST_BEFORE, svc, func(ctx context.Context) error {
			return svc.ListBefore(types.NewServiceContext(nil, ctx, consts.PHASE_LIST_BEFORE), &data)
		}))
		require.NotContains(t, oteltest.EndedNames(recorder), "service.FactoryRouteModel.ListBefore")
	})

	t.Run("an overridden hook gets a span and its no-op partner does not", func(t *testing.T) {
		svc := &factoryListBeforeService{}
		data := make([]*factoryRouteModel, 0)
		require.NoError(t, meta.traceServiceHook(context.Background(), consts.PHASE_LIST_BEFORE, svc, func(ctx context.Context) error {
			return svc.ListBefore(types.NewServiceContext(nil, ctx, consts.PHASE_LIST_BEFORE), &data)
		}))
		require.NoError(t, meta.traceServiceHook(context.Background(), consts.PHASE_LIST_AFTER, svc, func(ctx context.Context) error {
			return svc.ListAfter(types.NewServiceContext(nil, ctx, consts.PHASE_LIST_AFTER), &data)
		}))
		names := oteltest.EndedNames(recorder)
		require.Contains(t, names, "service.FactoryRouteModel.ListBefore")
		require.NotContains(t, names, "service.FactoryRouteModel.ListAfter")
	})
}
