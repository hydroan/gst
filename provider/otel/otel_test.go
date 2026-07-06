package otel

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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
	require.Equal(t, sdktrace.DefaultMaxQueueSize, cfg.BSPMaxQueueSize)
	require.Equal(t, sdktrace.DefaultMaxExportBatchSize, cfg.BSPMaxExportBatchSize)
	require.Equal(t, time.Duration(sdktrace.DefaultScheduleDelay)*time.Millisecond, cfg.BSPScheduleDelay)
	require.Equal(t, time.Duration(sdktrace.DefaultExportTimeout)*time.Millisecond, cfg.BSPExportTimeout)
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

	require.Equal(t, sdktrace.RecordAndSample, sampleDecision(context.Background(), t, sampler))
	require.Equal(t, sdktrace.RecordAndSample, sampleDecision(parentContext(t, true), t, sampler))
	require.Equal(t, sdktrace.Drop, sampleDecision(parentContext(t, false), t, sampler))
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

func TestResolveInstanceIDPrefersHostname(t *testing.T) {
	require.Equal(t, "node-1", resolveInstanceID("node-1", nil))
}

func TestResolveInstanceIDFallsBackToGeneratedIDWhenHostnameUnavailable(t *testing.T) {
	require.NotEmpty(t, resolveInstanceID("", errors.New("lookup failed")))
	require.NotEmpty(t, resolveInstanceID("", nil))
}

func TestResourceAttributesIncludesServiceAndEnvironmentInfo(t *testing.T) {
	t.Cleanup(func() {
		config.App.AppInfo.Version = ""
		config.App.Mode = ""
	})

	config.App.AppInfo.Version = "v1.2.3"
	config.App.Mode = config.Prod

	attrs := resourceAttributes(config.OTEL{ServiceName: "dice"})

	values := make(map[attribute.Key]attribute.Value, len(attrs))
	for _, kv := range attrs {
		values[kv.Key] = kv.Value
	}

	require.Equal(t, "dice", values[semconv.ServiceNameKey].AsString())
	require.Equal(t, "v1.2.3", values[semconv.ServiceVersionKey].AsString())
	require.Equal(t, "prod", values[semconv.DeploymentEnvironmentKey].AsString())
	require.NotEmpty(t, values[semconv.ServiceInstanceIDKey].AsString())
}

func TestIsSpanRecording(t *testing.T) {
	require.False(t, IsSpanRecording(nil))
	require.False(t, IsSpanRecording(&attributeCountingSpan{}))
	require.True(t, IsSpanRecording(&attributeCountingSpan{recording: true}))
}

func TestAddSpanTagsSetsAttributesInOneBatch(t *testing.T) {
	span := &attributeCountingSpan{recording: true}

	AddSpanTags(span, map[string]any{
		"bool":    true,
		"float":   1.2,
		"int":     1,
		"int64":   int64(2),
		"string":  "value",
		"unknown": struct{}{},
	})

	require.Equal(t, 1, span.setAttributesCalls)
	require.Len(t, span.attributes, 6)
	require.Contains(t, span.attributes, attribute.Bool("bool", true))
	require.Contains(t, span.attributes, attribute.Float64("float", 1.2))
	require.Contains(t, span.attributes, attribute.Int("int", 1))
	require.Contains(t, span.attributes, attribute.Int64("int64", 2))
	require.Contains(t, span.attributes, attribute.String("string", "value"))
	require.Contains(t, span.attributes, attribute.String("unknown", "unsupported_type"))
}

func TestFrameworkSpanNameUsesDottedGoNames(t *testing.T) {
	tests := []struct {
		name      string
		component string
		resource  string
		operation string
		want      string
	}{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, FrameworkSpanName(tt.component, tt.resource, tt.operation))
		})
	}
}

func TestOperationSpanNameUsesDottedGoNames(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, OperationSpanName(tt.component, tt.operation))
		})
	}
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

func sampleDecision(parent context.Context, t *testing.T, sampler sdktrace.Sampler) sdktrace.SamplingDecision {
	t.Helper()

	traceID, err := oteltrace.TraceIDFromHex("11111111111111111111111111111111")
	require.NoError(t, err)

	result := sampler.ShouldSample(sdktrace.SamplingParameters{
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
