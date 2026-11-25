package tracing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/instance"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestNormalizeConfigUsesOTELDefaults(t *testing.T) {
	cfg, err := normalizeConfig(config.OTEL{})
	require.NoError(t, err)

	require.Equal(t, config.OTLPProtocolHTTPProtobuf, cfg.ExporterOTLPProtocol)
	require.Equal(t, "http://localhost:4318/v1/traces", cfg.ExporterOTLPTracesEndpoint)
	require.Equal(t, config.OTLPCompressionNone, cfg.ExporterOTLPCompression)
	require.Equal(t, config.TracesSamplerParentBasedAlwaysOn, cfg.TracesSampler)
	require.Empty(t, cfg.TracesSamplerArg)
	require.Equal(t, trace.DefaultMaxQueueSize, cfg.BSPMaxQueueSize)
	require.Equal(t, trace.DefaultMaxExportBatchSize, cfg.BSPMaxExportBatchSize)
	require.Equal(t, time.Duration(trace.DefaultScheduleDelay)*time.Millisecond, cfg.BSPScheduleDelay)
	require.Equal(t, time.Duration(trace.DefaultExportTimeout)*time.Millisecond, cfg.BSPExportTimeout)
}

func TestNormalizeConfigUsesGRPCDefaultEndpoint(t *testing.T) {
	cfg, err := normalizeConfig(config.OTEL{
		ExporterOTLPProtocol: config.OTLPProtocolGRPC,
	})
	require.NoError(t, err)

	require.Equal(t, "http://localhost:4317", cfg.ExporterOTLPTracesEndpoint)
}

func TestNewSamplerUsesParentBasedTraceIDRatio(t *testing.T) {
	sampler, err := newSampler(config.OTEL{
		TracesSampler:    config.TracesSamplerParentBasedTraceIDRatio,
		TracesSamplerArg: "1",
	})
	require.NoError(t, err)

	require.Equal(t, trace.RecordAndSample, sampleDecision(context.Background(), t, sampler))
	require.Equal(t, trace.RecordAndSample, sampleDecision(parentContext(t, true), t, sampler))
	require.Equal(t, trace.Drop, sampleDecision(parentContext(t, false), t, sampler))
}

func TestNewSamplerRejectsInvalidTraceIDRatio(t *testing.T) {
	for _, arg := range []string{"-0.1", "1.1", "not-a-number"} {
		t.Run(arg, func(t *testing.T) {
			_, err := newSampler(config.OTEL{
				TracesSampler:    config.TracesSamplerParentBasedTraceIDRatio,
				TracesSamplerArg: arg,
			})
			require.Error(t, err)
		})
	}
}

func TestNormalizeConfigRejectsOversizedExportBatch(t *testing.T) {
	_, err := normalizeConfig(config.OTEL{
		BSPMaxQueueSize:       100,
		BSPMaxExportBatchSize: 101,
	})
	require.Error(t, err)
}

func TestResolveServiceVersionPrefersAppVersion(t *testing.T) {
	t.Cleanup(func() {
		config.App.AppInfo.Version = ""
		config.App.AppInfo.GitCommit = ""
	})

	config.App.AppInfo.Version = "v1.2.3"
	config.App.AppInfo.GitCommit = "abc123"
	require.Equal(t, "v1.2.3", resolveServiceVersion())
}

func TestResolveServiceVersionFallsBackToGitCommit(t *testing.T) {
	t.Cleanup(func() {
		config.App.AppInfo.Version = ""
		config.App.AppInfo.GitCommit = ""
	})

	config.App.AppInfo.Version = ""
	config.App.AppInfo.GitCommit = "abc123"
	require.Equal(t, "abc123", resolveServiceVersion())
}

func TestResolveServiceVersionFallsBackToUnknown(t *testing.T) {
	t.Cleanup(func() {
		config.App.AppInfo.Version = ""
		config.App.AppInfo.GitCommit = ""
	})

	config.App.AppInfo.Version = ""
	config.App.AppInfo.GitCommit = ""
	require.Equal(t, "unknown", resolveServiceVersion())
}

// TestResourceAttributesNameTheServiceEnvironmentAndInstance proves the
// resource carries what tells traces apart: the service, its version, the
// environment, and the process identity as the instance — the same identity
// the log entries carry — with the hostname beside it.
func TestResourceAttributesNameTheServiceEnvironmentAndInstance(t *testing.T) {
	t.Cleanup(func() {
		config.App.AppInfo.Version = ""
		config.App.Mode = ""
	})

	config.App.AppInfo.Version = "v1.2.3"
	config.App.Mode = config.Prod

	attrs := resourceAttributes(config.OTEL{ServiceName: "sample-service"})

	values := make(map[attribute.Key]attribute.Value, len(attrs))
	for _, kv := range attrs {
		values[kv.Key] = kv.Value
	}

	require.Equal(t, "sample-service", values[semconv.ServiceNameKey].AsString())
	require.Equal(t, "v1.2.3", values[semconv.ServiceVersionKey].AsString())
	require.Equal(t, "prod", values[semconv.DeploymentEnvironmentKey].AsString())
	require.Equal(t, instance.ID(), values[semconv.ServiceInstanceIDKey].AsString())
	require.Equal(t, instance.Hostname(), values[semconv.HostNameKey].AsString())
}

func TestIsSpanRecording(t *testing.T) {
	require.False(t, IsSpanRecording(nil))
	require.False(t, IsSpanRecording(&attributeCountingSpan{}))
	require.True(t, IsSpanRecording(&attributeCountingSpan{recording: true}))
}

func TestRecordErrorRecordsErrorOriginStackTrace(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "error with stack trace",
			err:  newStackTracedError(),
		},
		{
			name: "wrapped error keeps origin stack trace",
			err:  errors.Wrap(newStackTracedError(), "wrapped"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := recordErrorOnSpan(t, tt.err)
			require.Equal(t, codes.Error, span.Status().Code)

			stackTrace := exceptionStackTrace(t, span)
			require.Contains(t, stackTrace, "newStackTracedError")
			require.Contains(t, stackTrace, "tracing_internal_test.go")
		})
	}
}

func TestRecordErrorFallsBackToRecordSiteStackTrace(t *testing.T) {
	span := recordErrorOnSpan(t, plainError{})
	require.Equal(t, codes.Error, span.Status().Code)

	stackTrace := exceptionStackTrace(t, span)
	require.Contains(t, stackTrace, "TestRecordErrorFallsBackToRecordSiteStackTrace")
}

// TestRecordPanicRecordsThePanicOnTheSpan pins what a recovered panic leaves
// on a recording span, whatever was recovered: the exception event of the
// error describing it, whose stack starts at the caller that recovered it,
// RecordPanic being no frame of it; the Error status; and the two attributes
// that tell a panic from any other failure. The error is returned, for the
// caller to report further.
func TestRecordPanicRecordsThePanicOnTheSpan(t *testing.T) {
	for _, tt := range []struct {
		name      string
		recovered any
		text      string
	}{
		{name: "a string", recovered: "boom", text: "boom"},
		{name: "an error", recovered: errors.New("sample failure"), text: "sample failure"},
		{name: "a number", recovered: 42, text: "42"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

			_, span := provider.Tracer("test").Start(context.Background(), "operation")
			err := RecordPanic(span, tt.recovered)
			span.End()

			require.EqualError(t, err, "panic recovered: "+tt.text)
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			require.Equal(t, codes.Error, spans[0].Status().Code)
			require.Equal(t, "panic recovered: "+tt.text, spans[0].Status().Description)
			attrs := make(map[attribute.Key]any, len(spans[0].Attributes()))
			for _, attr := range spans[0].Attributes() {
				attrs[attr.Key] = attr.Value.AsInterface()
			}
			require.Equal(t, map[attribute.Key]any{"error.panic": true, "error.recovered": tt.text}, attrs)
			origin, _, _ := strings.Cut(exceptionStackTrace(t, spans[0]), "\n")
			require.Contains(t, origin, "TestRecordPanicRecordsThePanicOnTheSpan", "the stack starts at the caller that recovered the panic")
		})
	}
}

// TestRecordPanicReturnsTheErrorWithNoSpanToRecordOn pins that a span not
// recording, or none at all, is left alone, and the error describing the
// panic is returned all the same.
func TestRecordPanicReturnsTheErrorWithNoSpanToRecordOn(t *testing.T) {
	idle := &attributeCountingSpan{}
	require.EqualError(t, RecordPanic(idle, "boom"), "panic recovered: boom")
	require.Zero(t, idle.setAttributesCalls)

	require.EqualError(t, RecordPanic(nil, "boom"), "panic recovered: boom")
}

func TestResourceSpanNameUsesDottedGoNames(t *testing.T) {
	tests := []struct {
		name      string
		component string
		resource  string
		operation string
		want      string
	}{
		{
			name:      "database get",
			component: "database",
			resource:  "Record",
			operation: "Get",
			want:      "database.Record.Get",
		},
		{
			name:      "controller create",
			component: "controller",
			resource:  "RoleBinding",
			operation: "Create",
			want:      "controller.RoleBinding.Create",
		},
		{
			name:      "model create after",
			component: "model",
			resource:  "RoleBinding",
			operation: "CreateAfter",
			want:      "model.RoleBinding.CreateAfter",
		},
		{
			name:      "service delete many after",
			component: "service",
			resource:  "AdminUserSession",
			operation: "DeleteManyAfter",
			want:      "service.AdminUserSession.DeleteManyAfter",
		},
		{
			name:      "lower camel inputs",
			component: "database",
			resource:  "roleBinding",
			operation: "createAfter",
			want:      "database.RoleBinding.CreateAfter",
		},
		{
			name:      "component in camel case",
			component: "HTTPServer",
			resource:  "Record",
			operation: "Get",
			want:      "http_server.Record.Get",
		},
		{
			name:      "component with a separator",
			component: "http-server",
			resource:  "Record",
			operation: "Get",
			want:      "http_server.Record.Get",
		},
		{
			name:      "no resource",
			component: "database",
			resource:  "",
			operation: "get",
			want:      "database.Get",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ResourceSpanName(tt.component, tt.resource, tt.operation))
		})
	}
}

func TestSpanNameUsesDottedGoNames(t *testing.T) {
	tests := []struct {
		name      string
		component string
		operation string
		want      string
	}{
		{
			name:      "middleware",
			component: "middleware",
			operation: "IAMSession",
			want:      "middleware.IAMSession",
		},
		{
			name:      "cache",
			component: "cache",
			operation: "get",
			want:      "cache.Get",
		},
		{
			name:      "rbac",
			component: "rbac",
			operation: "assign_role",
			want:      "rbac.AssignRole",
		},
		{
			name:      "qualified function name",
			component: "middleware",
			operation: "github.com/x/y.Middleware",
			want:      "middleware.GithubComXYMiddleware",
		},
		{
			name:      "no component",
			component: "",
			operation: "get",
			want:      "Get",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, SpanName(tt.component, tt.operation))
		})
	}
}

// TestSpanNameSegmentsFitTheCapacityInTheWorstCase pins spanNameSegmentCap
// to the longest name there is, a component, a resource and an operation:
// its segments fill the list joinSpanName sizes exactly, so building it
// allocates the name's own text and nothing else, where a segment more
// would grow the list onto the heap.
func TestSpanNameSegmentsFitTheCapacityInTheWorstCase(t *testing.T) {
	name := ResourceSpanName("service", "AdminUserSession", "DeleteManyAfter")
	require.Len(t, strings.Split(name, "."), spanNameSegmentCap)

	// AllocsPerRun averages over whole allocations, so the count is an
	// integer already.
	allocs := int(testing.AllocsPerRun(100, func() {
		_ = ResourceSpanName("service", "AdminUserSession", "DeleteManyAfter")
	}))
	require.Equal(t, 1, allocs)
}

// plainError is an error without any embedded stack trace.
type plainError struct{}

func (plainError) Error() string { return "plain failure" }

// newStackTracedError creates an error whose stack trace points at this
// helper, so tests can assert the error origin is recorded.
func newStackTracedError() error {
	return errors.New("stack traced failure")
}

// recordErrorOnSpan runs RecordError against a real recording span and
// returns the ended span for assertions.
func recordErrorOnSpan(t *testing.T, err error) trace.ReadOnlySpan {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	_, span := provider.Tracer("test").Start(context.Background(), "operation")
	RecordError(span, err)
	span.End()

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	return spans[0]
}

func exceptionStackTrace(t *testing.T, span trace.ReadOnlySpan) string {
	t.Helper()

	events := span.Events()
	require.Len(t, events, 1)
	require.Equal(t, semconv.ExceptionEventName, events[0].Name)

	for _, attr := range events[0].Attributes {
		if attr.Key == semconv.ExceptionStacktraceKey {
			return attr.Value.AsString()
		}
	}
	require.Failf(t, "missing attribute", "exception event has no %s attribute", semconv.ExceptionStacktraceKey)
	return ""
}

type attributeCountingSpan struct {
	oteltrace.Span

	recording          bool
	setAttributesCalls int
	attributes         []attribute.KeyValue
}

func (s *attributeCountingSpan) IsRecording() bool {
	return s.recording
}

func (s *attributeCountingSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.setAttributesCalls++
	s.attributes = append(s.attributes, kv...)
}

func (s *attributeCountingSpan) SetStatus(code codes.Code, description string) {}

func sampleDecision(parent context.Context, t *testing.T, sampler trace.Sampler) trace.SamplingDecision {
	t.Helper()

	traceID, err := oteltrace.TraceIDFromHex("11111111111111111111111111111111")
	require.NoError(t, err)

	result := sampler.ShouldSample(trace.SamplingParameters{
		ParentContext: parent,
		TraceID:       traceID,
		Name:          "GET /api/ping",
		Kind:          oteltrace.SpanKindServer,
	})
	return result.Decision
}

func parentContext(t *testing.T, sampled bool) context.Context {
	t.Helper()

	traceID, err := oteltrace.TraceIDFromHex("22222222222222222222222222222222")
	require.NoError(t, err)

	spanID, err := oteltrace.SpanIDFromHex("3333333333333333")
	require.NoError(t, err)

	var flags oteltrace.TraceFlags
	if sampled {
		flags = oteltrace.FlagsSampled
	}

	spanContext := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: flags,
		Remote:     true,
	})
	return oteltrace.ContextWithRemoteSpanContext(context.Background(), spanContext)
}
