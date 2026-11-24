package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func TestTracingUsesIncomingTraceparent(t *testing.T) {
	setupTracingTest(t)

	const incomingTraceID = "11111111111111111111111111111111"

	router := gin.New()
	router.Use(tracing())
	router.GET("/api/ping", func(c *gin.Context) {
		spanContext := trace.SpanFromContext(c.Request.Context()).SpanContext()
		require.True(t, spanContext.HasTraceID())
		require.Equal(t, incomingTraceID, spanContext.TraceID().String())
		require.Equal(t, incomingTraceID, c.GetString(execctx.KeyTraceID))
		require.Equal(t, incomingTraceID, execctx.FromContext(c.Request.Context()).TraceID)
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.Header.Set("Traceparent", "00-"+incomingTraceID+"-2222222222222222-01")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, incomingTraceID, w.Header().Get(execctx.HeaderTraceID))
}

func TestTracingUsesIncomingTraceIDHeader(t *testing.T) {
	setupTracingTest(t)

	const incomingTraceID = "33333333333333333333333333333333"

	router := gin.New()
	router.Use(tracing())
	router.GET("/api/ping", func(c *gin.Context) {
		spanContext := trace.SpanFromContext(c.Request.Context()).SpanContext()
		require.True(t, spanContext.HasTraceID())
		require.Equal(t, incomingTraceID, spanContext.TraceID().String())
		require.Equal(t, incomingTraceID, c.GetString(execctx.KeyTraceID))
		require.Equal(t, incomingTraceID, execctx.FromContext(c.Request.Context()).TraceID)
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.Header.Set(execctx.HeaderTraceID, incomingTraceID)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, incomingTraceID, w.Header().Get(execctx.HeaderTraceID))
}

// TestTracingHoldsTheTraceIDHeaderToOneRuleWhenOTELDisabled pins the branch
// with tracing off: the middleware reads X-Trace-ID the way the propagator
// does with tracing on (see execctx.TraceIDOf), keeping a W3C trace id
// and replacing anything else, an absent header included, with a new id of
// the same form; the id it stamps on the context is the one it publishes
// in the gin context and hands the caller back, so the annotations
// downstream carry the id the caller sees; and the span id is new either
// way.
func TestTracingHoldsTheTraceIDHeaderToOneRuleWhenOTELDisabled(t *testing.T) {
	original := config.App.OTEL.Enabled
	config.App.OTEL.Enabled = false
	t.Cleanup(func() { config.App.OTEL.Enabled = original })

	const sent = "4bf92f3577b34da6a3ce929d0e0e4736"
	for name, tt := range map[string]struct{ header, want string }{
		"a W3C trace id is kept":         {header: sent, want: sent},
		"a bare word is replaced":        {header: "trace-1"},
		"an absent header gets a new id": {},
	} {
		t.Run(name, func(t *testing.T) {
			var stamped, published, spanID string
			router := gin.New()
			router.Use(tracing())
			router.GET("/api/ping", func(c *gin.Context) {
				stamped = execctx.FromContext(c.Request.Context()).TraceID
				published = c.GetString(execctx.KeyTraceID)
				spanID = c.GetString(execctx.KeySpanID)
				c.Status(http.StatusNoContent)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
			if tt.header != "" {
				req.Header.Set(execctx.HeaderTraceID, tt.header)
			}
			router.ServeHTTP(w, req)

			require.Regexp(t, `^[0-9a-f]{32}$`, stamped)
			if tt.want != "" {
				require.Equal(t, tt.want, stamped)
			} else {
				require.NotEqual(t, tt.header, stamped)
			}
			require.Equal(t, stamped, published)
			require.Equal(t, stamped, w.Header().Get(execctx.HeaderTraceID))
			require.Regexp(t, `^[0-9a-f]{16}$`, spanID)
		})
	}
}

// TestTracingReplacesATraceIDHeaderThatIsNotOne pins the same rule with
// tracing on: the propagator ignores a header that is not a W3C trace id,
// the server span brings a new one, and that is what the caller is handed
// back.
func TestTracingReplacesATraceIDHeaderThatIsNotOne(t *testing.T) {
	setupTracingTest(t)

	router := gin.New()
	router.Use(tracing())
	router.GET("/api/ping", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.Header.Set(execctx.HeaderTraceID, "trace-1")
	router.ServeHTTP(w, req)

	got := w.Header().Get(execctx.HeaderTraceID)
	require.Regexp(t, `^[0-9a-f]{32}$`, got)
	require.NotEqual(t, "trace-1", got)
}

func TestTracingSkipsRecordingOnlyStateWhenSamplerDrops(t *testing.T) {
	setupTracingTest(t, oteltest.WithSampler(config.TracesSamplerAlwaysOff))

	router := gin.New()
	router.Use(tracing())
	router.GET("/api/ping", func(c *gin.Context) {
		span := trace.SpanFromContext(c.Request.Context())
		require.True(t, span.SpanContext().HasTraceID())
		require.False(t, span.IsRecording())

		_, exists := c.Get("request_start_time")
		require.False(t, exists)
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.NotEmpty(t, w.Header().Get(execctx.HeaderTraceID))
}

// TestTracingNamesTheSpanAfterTheRoutePattern pins the name of the request
// root span to the OpenTelemetry HTTP semantic conventions: the method and
// the route pattern gin matched, so every request to one route shares one
// name, and the method alone for a request no route matched, whose path is
// whatever the client sent and would give a tracing backend one operation
// per path. That path stays on the span as url.path.
func TestTracingNamesTheSpanAfterTheRoutePattern(t *testing.T) {
	setupTracingTest(t)
	recorder := oteltest.Record(t)

	router := gin.New()
	router.Use(tracing())
	router.GET("/api/samples/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	paths := []string{"/api/samples/1", "/api/samples/2", "/unregistered/1", "/unregistered/2"}
	for _, path := range paths {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	require.Equal(t,
		[]string{"GET /api/samples/:id", "GET /api/samples/:id", "GET", "GET"},
		oteltest.EndedNames(recorder),
	)

	targets := make([]string, 0, len(paths))
	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			if attr.Key == "url.path" {
				targets = append(targets, attr.Value.AsString())
			}
		}
	}
	require.Equal(t, paths, targets)
}

// TestTracingSetsTheSpanStatusTheWayTheGRPCLineDoes pins the status of the
// request root span to the OpenTelemetry HTTP semantic conventions, which
// otelgrpc holds the gRPC line's server spans to as well: a successful
// answer and a client error leave the status Unset, a server error sets
// Error, and so does a request the handler reported an error on, whatever
// its status code; Ok is never set, the SDK treating it as final, so no
// later error is masked by it.
func TestTracingSetsTheSpanStatusTheWayTheGRPCLineDoes(t *testing.T) {
	setupTracingTest(t)
	recorder := oteltest.Record(t)

	router := gin.New()
	router.Use(tracing())
	router.GET("/api/ok", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/api/missing", func(c *gin.Context) { c.Status(http.StatusNotFound) })
	router.GET("/api/failed", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })
	router.GET("/api/reported", func(c *gin.Context) {
		_ = c.Error(errors.New("sample failure"))
		c.Status(http.StatusOK)
	})

	for _, tc := range []struct {
		route string
		code  codes.Code
	}{
		{"/api/ok", codes.Unset},
		{"/api/missing", codes.Unset},
		{"/api/failed", codes.Error},
		{"/api/reported", codes.Error},
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.route, nil))

		span := oteltest.EndedNamed(t, recorder, "GET "+tc.route)
		require.Equal(t, tc.code, span.Status().Code, tc.route)
	}
}

func TestTracingMarksHTTPSpanAsRequestRoot(t *testing.T) {
	source := readMiddlewareSource(t, "tracing.go")
	require.Contains(t, source, "ctx = gstotel.ContextWithRequestRootSpan(ctx)")
}

// TestTracingDescribesTheRequestByTheHTTPConventions pins the attributes of
// the request root span to the names and types the OpenTelemetry HTTP
// semantic conventions give them: the method, the path and the query string
// apart, the scheme the client used, the Host split into address and port,
// the route, the user agent, the client address and the body sizes in bytes.
// What a request or its answer does not carry — a query string, a port in
// the Host, a matched route, a user agent, a body — is left off the span
// rather than recorded empty, and no header is captured, the content type
// included.
func TestTracingDescribesTheRequestByTheHTTPConventions(t *testing.T) {
	setupTracingTest(t)
	recorder := oteltest.Record(t)

	router := gin.New()
	router.Use(tracing())
	router.POST("/api/samples/:id", func(c *gin.Context) { c.String(http.StatusCreated, "created") })

	t.Run("a request carrying everything the span describes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/samples/7?_page=2&name=a", strings.NewReader("{}"))
		req.Host = "sample.test:8443"
		req.RemoteAddr = "192.0.2.10:54321"
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("User-Agent", "sample-agent/1.0")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(httptest.NewRecorder(), req)

		require.Equal(t, map[attribute.Key]any{
			"http.request.method":       "POST",
			"url.path":                  "/api/samples/7",
			"url.query":                 "_page=2&name=a",
			"url.scheme":                "https",
			"server.address":            "sample.test",
			"server.port":               int64(8443),
			"http.route":                "/api/samples/:id",
			"user_agent.original":       "sample-agent/1.0",
			"client.address":            "192.0.2.10",
			"http.request.body.size":    int64(2),
			"http.response.status_code": int64(http.StatusCreated),
			"http.response.body.size":   int64(len("created")),
		}, spanAttributeValues(t, oteltest.EndedNamed(t, recorder, "POST /api/samples/:id").Attributes()))
	})

	t.Run("a request carrying none of the optional parts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/unregistered", nil)
		req.Host = "sample.test"
		req.RemoteAddr = "192.0.2.10:54321"
		router.ServeHTTP(httptest.NewRecorder(), req)

		require.Equal(t, map[attribute.Key]any{
			"http.request.method":       "DELETE",
			"url.path":                  "/unregistered",
			"url.scheme":                "http",
			"server.address":            "sample.test",
			"client.address":            "192.0.2.10",
			"http.response.status_code": int64(http.StatusNotFound),
		}, spanAttributeValues(t, oteltest.EndedNamed(t, recorder, "DELETE").Attributes()))
	})
}

// TestSplitHostPort pins how the Host a request names its server by is split
// into the address and the port the span carries apart: the port a Host
// names is read off it, brackets come off an IPv6 address, and a Host naming
// no port, or one that is no port number, gives no port.
func TestSplitHostPort(t *testing.T) {
	for _, tt := range []struct {
		hostport string
		host     string
		port     int
	}{
		{"sample.test", "sample.test", 0},
		{"sample.test:8080", "sample.test", 8080},
		{"192.0.2.10:443", "192.0.2.10", 443},
		{"[2001:db8::1]:8080", "2001:db8::1", 8080},
		{"[2001:db8::1]", "2001:db8::1", 0},
		{"sample.test:", "sample.test", 0},
		{"sample.test:http", "sample.test", 0},
		{"sample.test:70000", "sample.test", 0},
		{"2001:db8::1", "2001:db8::1", 0},
		{"", "", 0},
	} {
		host, port := splitHostPort(tt.hostport)
		require.Equal(t, tt.host, host, tt.hostport)
		require.Equal(t, tt.port, port, tt.hostport)
	}
}

// TestTracingSpanAttributesFitTheCapacityInTheWorstCase pins the two batches
// the root span carries — a request with a query string, a port in its Host,
// a user agent and a body, a failed response with a body — plus one
// attribute per error the handler reported, so an attribute added without
// bumping its capacity fails here instead of regrowing the slice on every
// traced request.
func TestTracingSpanAttributesFitTheCapacityInTheWorstCase(t *testing.T) {
	setupTracingTest(t)
	recorder := oteltest.Record(t)

	router := gin.New()
	router.Use(tracing())
	router.POST("/api/samples", func(c *gin.Context) {
		_ = c.Error(errors.New("first sample failure"))
		_ = c.Error(errors.New("second sample failure"))
		c.String(http.StatusInternalServerError, "failed")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/samples?name=a", strings.NewReader("{}"))
	req.Host = "sample.test:8080"
	req.Header.Set("User-Agent", "sample-agent/1.0")
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)

	span := oteltest.EndedNamed(t, recorder, "POST /api/samples")
	// Each batch is pinned by the count under its own keys, so one batch
	// growing past its capacity is not hidden by the other shrinking. The
	// error batch adds one attribute per error and re-sets the error flag the
	// response batch already carries; a span keeps one value per key.
	const errorsReported = 2
	batches := oteltest.AttributesByKey(t, span.Attributes(),
		[]attribute.Key{"http.request.method", "url.path", "url.query", "url.scheme", "server.address", "server.port", "http.route", "user_agent.original", "client.address", "http.request.body.size"},
		[]attribute.Key{"http.response.status_code", "http.response.body.size", "http.duration_ms", "error"},
		[]attribute.Key{"error.0", "error.1"},
	)
	require.Len(t, batches[0], requestSpanAttrCap, "the worst case must fill the request batch exactly: a new attribute bumps requestSpanAttrCap")
	require.Len(t, batches[1], responseSpanAttrCap, "the worst case must fill the response batch exactly: a new attribute bumps responseSpanAttrCap")
	require.Len(t, batches[2], errorsReported, "one attribute per error the handler reported")
}

// setupTracingTest enables real tracing for one middleware test and puts gin
// into test mode; opts adjust the sampler or the exporter endpoint.
func setupTracingTest(t *testing.T, opts ...oteltest.Option) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oteltest.Enable(t, opts...)
}

func readMiddlewareSource(t *testing.T, filename string) string {
	t.Helper()

	source, err := os.ReadFile(filename)
	require.NoError(t, err)
	return string(source)
}

// spanAttributeValues returns the attributes of a span as plain values by
// key, without the duration, which no two runs share; the duration must be
// there.
func spanAttributeValues(t *testing.T, attrs []attribute.KeyValue) map[attribute.Key]any {
	t.Helper()

	values := make(map[attribute.Key]any, len(attrs))
	for _, attr := range attrs {
		values[attr.Key] = attr.Value.AsInterface()
	}
	require.Contains(t, values, attribute.Key("http.duration_ms"))
	delete(values, "http.duration_ms")
	return values
}
