// Package otel provides OpenTelemetry tracing integration using OTLP exporters.
//
// Note: This package was originally designed for Jaeger integration, but since
// OpenTelemetry dropped support for the Jaeger exporter in July 2023, it now
// uses OTLP (OpenTelemetry Protocol) exporters instead. Jaeger officially
// accepts and recommends using OTLP for sending traces.
//
// Supported exporter types:
//   - http/protobuf: OTLP over HTTP using protobuf payloads.
//   - grpc: OTLP over gRPC.
//
// Full sampling configuration:
//
//	[otel]
//	enabled = true
//	service_name = demo
//	exporter_otlp_protocol = http/protobuf
//	exporter_otlp_traces_endpoint = http://localhost:4318/v1/traces
//	traces_sampler = parentbased_always_on
//
// Partial sampling configuration:
//
//	[otel]
//	enabled = true
//	service_name = demo
//	exporter_otlp_protocol = http/protobuf
//	exporter_otlp_traces_endpoint = http://localhost:4318/v1/traces
//	traces_sampler = parentbased_traceidratio
//	traces_sampler_arg = 0.1
//	bsp_max_queue_size = 2048
//	bsp_max_export_batch_size = 512
//	bsp_schedule_delay = 5s
//	bsp_export_timeout = 30s
//
// Use traces_sampler=parentbased_traceidratio with traces_sampler_arg between
// 0 and 1 to enable partial sampling while honoring upstream sampling decisions.
// For example, traces_sampler_arg=0.1 samples about 10% of root traces.
//
// The package uses OTLP exporters to send traces to Jaeger or other
// OTLP-compatible backends like Uptrace.
package otel

import (
	"context"
	"maps"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/stoewer/go-strcase"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	// "go.opentelemetry.io/otel/exporters/jaeger" // deprecated: use OTLP exporters instead
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

var (
	tracer         trace.Tracer
	tracerProvider *sdktrace.TracerProvider
	mu             sync.Mutex
	initialized    bool

	ErrOTELIsDisabled = errors.New("otel is disabled")
)

type requestRootSpanKey struct{}

// Init initializes the OpenTelemetry tracer with OTLP exporters.
// This function replaces the deprecated Jaeger exporter with OTLP exporters
// that are compatible with Jaeger and other tracing backends.
func Init() error {
	cfg, err := normalizeConfig(config.App.OTEL)
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		logger.OTEL.Info("otel tracing is disabled")
		return nil
	}

	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	// Route internal SDK errors (e.g. export failures) through the application
	// logger instead of the default stderr handler so they surface in the same
	// log pipeline as everything else.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.OTEL.Errorw("otel internal error", "err", err)
	}))

	// Create exporter
	exporter, err := newExporter(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create exporter")
	}

	// Create resource
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(resourceAttributes(cfg)...),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create resource")
	}

	// Create sampler
	sampler, err := newSampler(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create sampler")
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			exporter,
			newBatchSpanProcessorOptions(cfg)...,
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create tracer
	tracer = otel.Tracer(cfg.ServiceName)

	// Store tracer provider for cleanup
	tracerProvider = tp

	initialized = true
	logger.OTEL.Info(
		"otel tracing initialized",
		zap.String("service_name", cfg.ServiceName),
		zap.String("exporter_otlp_protocol", string(cfg.ExporterOTLPProtocol)),
		zap.String("traces_sampler", string(cfg.TracesSampler)),
	)

	return nil
}

// Close closes the OpenTelemetry tracer provider.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if !initialized || tracerProvider == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tracerProvider.Shutdown(ctx); err != nil {
		logger.OTEL.Errorw("failed to shutdown tracer provider", "err", err)
	}

	initialized = false
	tracerProvider = nil
	logger.OTEL.Info("otel tracer closed")
}

// GetTracer returns the global tracer
func GetTracer() trace.Tracer {
	if !initialized {
		return noop.NewTracerProvider().Tracer("noop")
	}
	return tracer
}

// IsEnabled returns whether OpenTelemetry tracing is enabled.
func IsEnabled() bool {
	return config.App.OTEL.Enabled && initialized
}

// FrameworkSpanName returns the canonical name for gst-owned resource spans.
// The format is component.GoModel.GoOperation so Jaeger labels map directly to
// the framework type and method names used in code.
func FrameworkSpanName(component string, resource string, operation string) string {
	return joinSpanName(component, resource, operation)
}

// OperationSpanName returns the canonical name for gst-owned operation spans
// that do not have a resource segment, such as middleware, cache, and RBAC.
func OperationSpanName(component string, operation string) string {
	return joinSpanName(component, operation)
}

// StartSpan starts a new span with the given name and options. The caller owns
// the returned span and must end it after the traced operation finishes.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if !IsEnabled() {
		return ctx, trace.SpanFromContext(ctx)
	}
	return tracer.Start(ctx, name, opts...) //nolint:spancheck // Caller receives and ends the returned span.
}

// SpanFromContext returns the span from the context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// IsSpanRecording reports whether span is active and records telemetry data.
func IsSpanRecording(span trace.Span) bool {
	return span != nil && span.IsRecording()
}

// ContextWithRequestRootSpan marks the current span as the root span for one HTTP request.
func ContextWithRequestRootSpan(ctx context.Context) context.Context {
	span := trace.SpanFromContext(ctx)
	if span == nil || !span.SpanContext().IsValid() {
		return ctx
	}
	return context.WithValue(ctx, requestRootSpanKey{}, span)
}

// RequestRootContext returns ctx with the request root span restored as the current span.
func RequestRootContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	span, ok := ctx.Value(requestRootSpanKey{}).(trace.Span)
	if !ok || span == nil || !span.SpanContext().IsValid() {
		return ctx
	}
	return trace.ContextWithSpan(ctx, span)
}

func joinSpanName(component string, symbols ...string) string {
	names := make([]string, 0, len(symbols)+1)
	if component = spanComponentName(component); component != "" {
		names = append(names, component)
	}
	for _, symbol := range symbols {
		if name := spanSymbolName(symbol); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ".")
}

func spanComponentName(segment string) string {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return ""
	}

	replacer := strings.NewReplacer(" ", "_", "-", "_", ".", "_", "/", "_")
	segment = replacer.Replace(segment)
	return strings.Trim(strcase.SnakeCase(segment), "_")
}

func spanSymbolName(segment string) string {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return ""
	}
	if !strings.ContainsAny(segment, " _-./") && segment[0] >= 'A' && segment[0] <= 'Z' {
		return segment
	}

	replacer := strings.NewReplacer(" ", "_", "-", "_", ".", "_", "/", "_")
	segment = replacer.Replace(segment)
	return strcase.UpperCamelCase(segment)
}

// normalizeConfig applies OTEL defaults and validates startup-only settings.
func normalizeConfig(cfg config.OTEL) (config.OTEL, error) {
	if cfg.ExporterOTLPProtocol == "" {
		cfg.ExporterOTLPProtocol = config.OTLPProtocolHTTPProtobuf
	}

	switch cfg.ExporterOTLPProtocol {
	case config.OTLPProtocolHTTPProtobuf:
		if cfg.ExporterOTLPTracesEndpoint == "" {
			cfg.ExporterOTLPTracesEndpoint = "http://localhost:4318/v1/traces"
		}
	case config.OTLPProtocolGRPC:
		if cfg.ExporterOTLPTracesEndpoint == "" {
			cfg.ExporterOTLPTracesEndpoint = "http://localhost:4317"
		}
	default:
		return cfg, errors.Errorf("unsupported otlp protocol: %s", cfg.ExporterOTLPProtocol)
	}

	if cfg.ExporterOTLPCompression == "" {
		cfg.ExporterOTLPCompression = config.OTLPCompressionNone
	}
	switch cfg.ExporterOTLPCompression {
	case config.OTLPCompressionNone, config.OTLPCompressionGzip:
	default:
		return cfg, errors.Errorf("unsupported otlp compression: %s", cfg.ExporterOTLPCompression)
	}

	if cfg.TracesSampler == "" {
		cfg.TracesSampler = config.TracesSamplerParentBasedAlwaysOn
	}
	if _, err := newSampler(cfg); err != nil {
		return cfg, err
	}

	if cfg.BSPMaxQueueSize <= 0 {
		cfg.BSPMaxQueueSize = sdktrace.DefaultMaxQueueSize
	}
	if cfg.BSPMaxExportBatchSize <= 0 {
		cfg.BSPMaxExportBatchSize = min(sdktrace.DefaultMaxExportBatchSize, cfg.BSPMaxQueueSize)
	}
	if cfg.BSPMaxExportBatchSize > cfg.BSPMaxQueueSize {
		return cfg, errors.Errorf("bsp max export batch size %d exceeds max queue size %d", cfg.BSPMaxExportBatchSize, cfg.BSPMaxQueueSize)
	}
	if cfg.BSPScheduleDelay <= 0 {
		cfg.BSPScheduleDelay = time.Duration(sdktrace.DefaultScheduleDelay) * time.Millisecond
	}
	if cfg.BSPExportTimeout <= 0 {
		cfg.BSPExportTimeout = time.Duration(sdktrace.DefaultExportTimeout) * time.Millisecond
	}

	return cfg, nil
}

// resourceAttributes builds the OpenTelemetry resource attributes describing
// this process, so traces can be told apart by environment and by instance
// when multiple replicas are running in production.
func resourceAttributes(cfg config.OTEL) []attribute.KeyValue {
	hostname, hostErr := os.Hostname()

	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(resolveServiceVersion()),
		semconv.DeploymentEnvironment(string(config.App.Mode)),
		semconv.ServiceInstanceID(resolveInstanceID(hostname, hostErr)),
	}
	if hostErr == nil && hostname != "" {
		attrs = append(attrs, semconv.HostName(hostname))
	}
	return attrs
}

// resolveServiceVersion returns the service.version resource attribute value.
// It prefers the resolved application version (populated from VCS build info,
// see config.AppInfo.setBuildInfo), falling back to the git commit hash and
// finally "unknown" when neither is available.
func resolveServiceVersion() string {
	if v := strings.TrimSpace(config.App.AppInfo.Version); v != "" {
		return v
	}
	if commit := strings.TrimSpace(config.App.AppInfo.GitCommit); commit != "" {
		return commit
	}
	return "unknown"
}

// resolveInstanceID returns the service.instance.id resource attribute value.
// It prefers the process hostname, which already uniquely identifies a
// container or pod in typical production deployments, falling back to a
// generated UUID when the hostname is unavailable.
func resolveInstanceID(hostname string, hostErr error) string {
	if hostErr == nil && strings.TrimSpace(hostname) != "" {
		return hostname
	}
	return uuid.NewString()
}

// newExporter creates an OTLP trace exporter based on startup configuration.
func newExporter(cfg config.OTEL) (sdktrace.SpanExporter, error) {
	switch cfg.ExporterOTLPProtocol {
	case config.OTLPProtocolHTTPProtobuf:
		opts := []otlptracehttp.Option{}
		if isEndpointURL(cfg.ExporterOTLPTracesEndpoint) {
			opts = append(opts, otlptracehttp.WithEndpointURL(cfg.ExporterOTLPTracesEndpoint))
		} else {
			opts = append(opts, otlptracehttp.WithEndpoint(cfg.ExporterOTLPTracesEndpoint), otlptracehttp.WithInsecure())
		}

		headers := make(map[string]string)
		maps.Copy(headers, cfg.ExporterOTLPHeaders)
		if len(headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(headers))
		}
		if cfg.ExporterOTLPCompression == config.OTLPCompressionGzip {
			opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
		}

		return otlptracehttp.New(context.Background(), opts...)

	case config.OTLPProtocolGRPC:
		opts := []otlptracegrpc.Option{}
		if isEndpointURL(cfg.ExporterOTLPTracesEndpoint) {
			opts = append(opts, otlptracegrpc.WithEndpointURL(cfg.ExporterOTLPTracesEndpoint))
		} else {
			opts = append(opts, otlptracegrpc.WithEndpoint(cfg.ExporterOTLPTracesEndpoint), otlptracegrpc.WithInsecure())
		}

		headers := make(map[string]string)
		maps.Copy(headers, cfg.ExporterOTLPHeaders)
		if len(headers) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(headers))
		}
		if cfg.ExporterOTLPCompression == config.OTLPCompressionGzip {
			opts = append(opts, otlptracegrpc.WithCompressor("gzip"))
		}

		return otlptracegrpc.New(context.Background(), opts...)

	default:
		return nil, errors.Errorf("unsupported otlp protocol: %s", cfg.ExporterOTLPProtocol)
	}
}

func isEndpointURL(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// newSampler creates an OpenTelemetry sampler from standard sampler names.
func newSampler(cfg config.OTEL) (sdktrace.Sampler, error) {
	sampler := config.TracesSampler(strings.ToLower(strings.TrimSpace(string(cfg.TracesSampler))))
	switch sampler {
	case config.TracesSamplerAlwaysOn:
		return sdktrace.AlwaysSample(), nil
	case config.TracesSamplerAlwaysOff:
		return sdktrace.NeverSample(), nil
	case config.TracesSamplerTraceIDRatio:
		ratio, err := traceIDRatioSamplerArg(cfg.TracesSamplerArg)
		if err != nil {
			return nil, err
		}
		return sdktrace.TraceIDRatioBased(ratio), nil
	case config.TracesSamplerParentBasedAlwaysOn:
		return sdktrace.ParentBased(sdktrace.AlwaysSample()), nil
	case config.TracesSamplerParentBasedAlwaysOff:
		return sdktrace.ParentBased(sdktrace.NeverSample()), nil
	case config.TracesSamplerParentBasedTraceIDRatio:
		ratio, err := traceIDRatioSamplerArg(cfg.TracesSamplerArg)
		if err != nil {
			return nil, err
		}
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio)), nil
	default:
		return nil, errors.Errorf("unsupported traces sampler: %s", cfg.TracesSampler)
	}
}

func traceIDRatioSamplerArg(arg string) (float64, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 1, nil
	}

	ratio, err := strconv.ParseFloat(arg, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse traces sampler arg")
	}
	if ratio < 0 || ratio > 1 {
		return 0, errors.Errorf("traces sampler arg must be between 0 and 1: %s", arg)
	}
	return ratio, nil
}

func newBatchSpanProcessorOptions(cfg config.OTEL) []sdktrace.BatchSpanProcessorOption {
	return []sdktrace.BatchSpanProcessorOption{
		sdktrace.WithMaxQueueSize(cfg.BSPMaxQueueSize),
		sdktrace.WithMaxExportBatchSize(cfg.BSPMaxExportBatchSize),
		sdktrace.WithBatchTimeout(cfg.BSPScheduleDelay),
		sdktrace.WithExportTimeout(cfg.BSPExportTimeout),
	}
}

// AddSpanTags adds tags to the current span
func AddSpanTags(span trace.Span, tags map[string]any) {
	if !IsSpanRecording(span) || len(tags) == 0 {
		return
	}

	attrs := make([]attribute.KeyValue, 0, len(tags))
	for key, value := range tags {
		switch v := value.(type) {
		case string:
			attrs = append(attrs, attribute.String(key, v))
		case int:
			attrs = append(attrs, attribute.Int(key, v))
		case int64:
			attrs = append(attrs, attribute.Int64(key, v))
		case float64:
			attrs = append(attrs, attribute.Float64(key, v))
		case bool:
			attrs = append(attrs, attribute.Bool(key, v))
		default:
			attrs = append(attrs, attribute.String(key, "unsupported_type"))
		}
	}
	span.SetAttributes(attrs...)
}

// AddSpanEvent adds an event to the current span
func AddSpanEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	if !IsSpanRecording(span) {
		return
	}
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// RecordError records an error in the current span
func RecordError(span trace.Span, err error) {
	if !IsSpanRecording(span) || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
