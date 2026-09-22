package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/sse"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// sseSampleService streams until its context ends and returns that ending,
// wrapped the way service code wraps an error.
type sseSampleService struct {
	serviceregistry.Base[*factoryRouteModel, *factoryRouteModel, *factoryRouteModel]
}

func (*sseSampleService) SSE(ctx *types.ServiceContext) error {
	<-ctx.Done()
	return errors.Wrap(ctx.Err(), "sample stream")
}

// TestSSEFactoryRecordsAStreamEndedByShutdownAsInterrupted proves the server
// shutting down ends an SSE request through its context, and that a stream
// ending that way is recorded as how it ended rather than as a failure: the
// controller span carries an interrupted event naming the shutdown, and
// neither span is marked as failed although the service returned the ending.
func TestSSEFactoryRecordsAStreamEndedByShutdownAsInterrupted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger.Controller = zap.New("")
	oteltest.Enable(t)
	recorder := oteltest.Record(t)

	registerTestService[*factoryRouteModel, *factoryRouteModel, *factoryRouteModel](consts.PHASE_SSE, "samples/stream", &sseSampleService{})
	engine := gin.New()
	engine.GET("/samples/stream", SSEFactory[*factoryRouteModel, *factoryRouteModel, *factoryRouteModel](&types.ControllerConfig[*factoryRouteModel]{Route: "samples/stream"}))

	shutdown, beginShutdown := context.WithCancel(t.Context())
	req := httptest.NewRequestWithContext(sse.WithServerShutdown(t.Context(), shutdown), http.MethodGet, "/samples/stream", nil)
	served := make(chan struct{})
	go func() {
		defer close(served)
		engine.ServeHTTP(httptest.NewRecorder(), req)
	}()
	beginShutdown()
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("the request must end when the server shuts down")
	}

	controllerSpan := oteltest.EndedNamed(t, recorder, "controller.FactoryRouteModel.SSE")
	require.Equal(t, []string{"server shutting down"}, eventReasons(controllerSpan, "interrupted"))
	require.Equal(t, codes.Unset, controllerSpan.Status().Code, "a stream ended by the shutdown is not a failure")
	serviceSpan := oteltest.EndedNamed(t, recorder, "service.FactoryRouteModel.SSE")
	require.Equal(t, codes.Unset, serviceSpan.Status().Code, "nor is it one on the service span")
}

// eventReasons returns the reason attribute of every event named name on span.
func eventReasons(span sdktrace.ReadOnlySpan, name string) []string {
	var reasons []string
	for _, event := range span.Events() {
		if event.Name != name {
			continue
		}
		for _, attr := range event.Attributes {
			if attr.Key == "reason" {
				reasons = append(reasons, attr.Value.AsString())
			}
		}
	}
	return reasons
}
