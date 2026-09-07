// Package oteltest turns real OpenTelemetry tracing on inside one framework
// test and reads back the spans it exports.
//
// It is the framework's own test support: no public package forwards it, so
// a project built on gst never sees it, and it can follow the framework's
// test needs without becoming a contract.
package oteltest

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	pkgzap "github.com/hydroan/gst/logger/zap"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
	"go.opentelemetry.io/otel/trace/noop"
)

// unreachableEndpoint is where the exporter is pointed by default: nothing
// listens there, so exports fail fast and the spans a test reads come from
// Record instead.
const unreachableEndpoint = "http://127.0.0.1:1/v1/traces"

// Option adjusts the tracing configuration Enable installs.
type Option func(*config.OTEL)

// WithSampler sets the sampler; the default samples every trace.
func WithSampler(sampler config.TracesSampler) Option {
	return func(c *config.OTEL) { c.TracesSampler = sampler }
}

// WithEndpoint points the exporter somewhere other than the unreachable
// default.
func WithEndpoint(endpoint string) Option {
	return func(c *config.OTEL) { c.ExporterOTLPTracesEndpoint = endpoint }
}

// Enable turns tracing on for the rest of the test. Only config.App.OTEL is
// replaced, so whatever else the test package configured — the database its
// TestMain connected, the redis handle — stays intact. The otel logger is
// silenced, the tracer provider is reinitialized, and all of it is restored
// when the test ends.
func Enable(t *testing.T, opts ...Option) {
	t.Helper()

	original := config.App.OTEL
	cfg := original
	cfg.Enabled = true
	cfg.ServiceName = "gst-test"
	cfg.ExporterOTLPProtocol = config.OTLPProtocolHTTPProtobuf
	cfg.ExporterOTLPTracesEndpoint = unreachableEndpoint
	cfg.ExporterOTLPCompression = config.OTLPCompressionNone
	cfg.TracesSampler = config.TracesSamplerParentBasedAlwaysOn
	cfg.BSPMaxQueueSize = 100
	cfg.BSPMaxExportBatchSize = 100
	cfg.BSPScheduleDelay = 10 * time.Millisecond
	cfg.BSPExportTimeout = time.Second
	for _, opt := range opts {
		opt(&cfg)
	}
	config.App.OTEL = cfg
	t.Cleanup(func() {
		config.App.OTEL = original
	})

	originalLogger := logger.OTEL
	logger.OTEL = pkgzap.New("/dev/null")
	t.Cleanup(func() {
		logger.OTEL = originalLogger
	})

	installSwitchOnce.Do(func() { otel.SetTracerProvider(globalSwitch) })

	gstotel.Close()
	require.NoError(t, gstotel.Init())
	t.Cleanup(func() {
		gstotel.Close()
	})

	provider, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	require.True(t, ok, "Init must have installed the SDK tracer provider")
	globalSwitch.target.Store(provider)
	t.Cleanup(func() {
		globalSwitch.target.Store(nil)
	})
}

// The otel global delegate binds to the first tracer provider ever set in the
// process and keeps forwarding there for the process's life; later
// SetTracerProvider calls only replace what GetTracerProvider returns. Every
// tracer obtained before that first call — the otelgorm plugin's, taken when
// the database was opened during bootstrap — therefore forwards to that first
// provider forever. Were it the SDK provider of one test, its Shutdown at
// cleanup would remove the processors but not stop span creation, and every
// statement of every later test and benchmark in the process would keep
// allocating recording spans nobody collects.
//
// Enable therefore makes the first provider a switch it owns: the switch
// forwards to the SDK provider of the running test and to a no-op provider
// between tests, so spans stop when the test does. Init's own
// SetTracerProvider still runs afterwards, so GetTracerProvider keeps
// returning the SDK provider Record relies on.
var (
	installSwitchOnce sync.Once
	globalSwitch      = &switchProvider{}
	idleTracer        = noop.NewTracerProvider().Tracer("gst-test-idle")
)

// switchProvider is the tracer provider the global delegate forwards to; its
// tracers consult the current target on every span start.
type switchProvider struct {
	embedded.TracerProvider
	target atomic.Pointer[sdktrace.TracerProvider]
}

func (p *switchProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return &switchTracer{provider: p, name: name, opts: opts}
}

// switchTracer forwards each span start to the switch's current target, and
// to a no-op tracer when there is none. The no-op path returns a
// non-recording span rather than the caller's own: instrumentation ends the
// span it finds in the context it gets back, and handing it its parent
// would let it end that instead.
type switchTracer struct {
	embedded.Tracer
	provider *switchProvider
	name     string
	opts     []trace.TracerOption
}

func (t *switchTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if target := t.provider.target.Load(); target != nil {
		return target.Tracer(t.name, t.opts...).Start(ctx, name, opts...) //nolint:spancheck // Caller receives and ends the returned span, as with gstotel.StartSpan.
	}
	return idleTracer.Start(ctx, name, opts...) //nolint:spancheck // Caller receives and ends the returned span, as with gstotel.StartSpan.
}

// Record attaches an in-memory span recorder to the SDK tracer provider that
// Enable installed and returns it; its Ended method lists the spans finished
// so far, which is how a test reads back what an operation exported.
func Record(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	provider, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	require.True(t, ok, "Enable must have installed the SDK tracer provider")
	recorder := tracetest.NewSpanRecorder()
	provider.RegisterSpanProcessor(recorder)
	return recorder
}

// EndedNames returns the names of every span the recorder has seen end.
func EndedNames(recorder *tracetest.SpanRecorder) []string {
	ended := recorder.Ended()
	names := make([]string, 0, len(ended))
	for _, span := range ended {
		names = append(names, span.Name())
	}
	return names
}

// EndedNamed returns the first ended span carrying name, failing the test
// when none was exported.
func EndedNamed(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range recorder.Ended() {
		if span.Name() == name {
			return span
		}
	}
	require.FailNow(t, "no ended span named "+name)
	return nil
}
