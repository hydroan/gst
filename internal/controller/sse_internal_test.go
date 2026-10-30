package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/sse"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
)

// sseSampleService streams until its context ends and returns that ending,
// wrapped the way service code wraps an error.
type sseSampleService struct {
	serviceregistry.Base[*handlerRouteModel, *handlerRouteModel, *handlerRouteModel]
}

func (*sseSampleService) SSE(ctx *types.ServiceContext) error {
	<-ctx.Done()
	return errors.Wrap(ctx.Err(), "sample stream")
}

// TestSSEHandlerRecordsAStreamEndedByShutdownAsInterrupted proves the server
// shutting down ends an SSE request through its context, and that a stream
// ending that way is recorded as how it ended rather than as a failure: the
// controller span carries an interrupted event naming the shutdown, and
// neither span is marked as failed although the service returned the ending.
func TestSSEHandlerRecordsAStreamEndedByShutdownAsInterrupted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oteltest.Enable(t)
	recorder := oteltest.Record(t)

	registerTestService[*handlerRouteModel, *handlerRouteModel, *handlerRouteModel](apiaction.SSE, "samples/stream", &sseSampleService{})
	engine := gin.New()
	engine.GET("/samples/stream", SSEHandler[*handlerRouteModel, *handlerRouteModel, *handlerRouteModel](&types.ControllerConfig[*handlerRouteModel]{Route: "samples/stream"}))

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

	controllerSpan := oteltest.EndedNamed(t, recorder, "controller.HandlerRouteModel.SSE")
	require.Equal(t, []string{"server shutting down"}, eventReasons(controllerSpan, "interrupted"))
	require.Equal(t, codes.Unset, controllerSpan.Status().Code, "a stream ended by the shutdown is not a failure")
	serviceSpan := oteltest.EndedNamed(t, recorder, "service.HandlerRouteModel.SSE")
	require.Equal(t, codes.Unset, serviceSpan.Status().Code, "nor is it one on the service span")
}

// TestSSEHandlerAnswersAStreamEndedBeforeItOpenedByWhatEndedIt pins the
// answer of a request whose stream ended before it opened, the service
// returning nothing but the ending: a client that went away is answered as
// canceled, the way every flow answers a client that canceled, and the
// server shutting down as unavailable, so that the client resumes on another
// replica.
func TestSSEHandlerAnswersAStreamEndedBeforeItOpenedByWhatEndedIt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	registerTestService[*handlerRouteModel, *handlerRouteModel, *handlerRouteModel](apiaction.SSE, "samples/ended-stream", &sseSampleService{})
	engine := gin.New()
	engine.GET("/samples/ended-stream", SSEHandler[*handlerRouteModel, *handlerRouteModel, *handlerRouteModel](&types.ControllerConfig[*handlerRouteModel]{Route: "samples/ended-stream"}))
	// serveEnded serves req, ends its stream through end and returns the
	// answer recorded.
	serveEnded := func(t *testing.T, req *http.Request, end func()) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		served := make(chan struct{})
		go func() {
			defer close(served)
			engine.ServeHTTP(rec, req)
		}()
		end()
		select {
		case <-served:
		case <-time.After(5 * time.Second):
			t.Fatal("the request must end with its stream")
		}
		return rec
	}

	t.Run("the client went away", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		rec := serveEnded(t, httptest.NewRequestWithContext(ctx, http.MethodGet, "/samples/ended-stream", nil), cancel)

		require.Equal(t, types.StatusClientClosedRequest, rec.Code)
		require.Equal(t, "context canceled", envelopeMsg(t, rec))
	})

	t.Run("the server shut down", func(t *testing.T) {
		shutdown, beginShutdown := context.WithCancel(t.Context())
		req := httptest.NewRequestWithContext(sse.WithServerShutdown(t.Context(), shutdown), http.MethodGet, "/samples/ended-stream", nil)
		rec := serveEnded(t, req, beginShutdown)

		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Equal(t, "the server is shutting down", envelopeMsg(t, rec))
	})
}

// envelopeMsg returns the msg of the envelope rec recorded.
func envelopeMsg(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Msg string `json:"msg"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope), rec.Body.String())
	return envelope.Msg
}

// eventReasons returns the reason attribute of every event named name on span.
func eventReasons(span trace.ReadOnlySpan, name string) []string {
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
