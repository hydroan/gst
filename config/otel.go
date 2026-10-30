package config

import (
	"time"

	"github.com/hydroan/gst/internal/types"
	"github.com/spf13/viper"
)

const (
	// OTEL_ENABLED enables OpenTelemetry tracing.
	OTEL_ENABLED = "OTEL_ENABLED"
	// OTEL_SERVICE_NAME configures the OpenTelemetry service.name resource attribute.
	OTEL_SERVICE_NAME = "OTEL_SERVICE_NAME"
	// OTEL_EXPORTER_OTLP_PROTOCOL configures the OTLP traces transport protocol.
	OTEL_EXPORTER_OTLP_PROTOCOL = "OTEL_EXPORTER_OTLP_PROTOCOL"
	// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT configures the trace-specific OTLP endpoint.
	OTEL_EXPORTER_OTLP_TRACES_ENDPOINT = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
	// OTEL_EXPORTER_OTLP_HEADERS configures headers sent with OTLP trace exports.
	OTEL_EXPORTER_OTLP_HEADERS = "OTEL_EXPORTER_OTLP_HEADERS"
	// OTEL_EXPORTER_OTLP_COMPRESSION configures OTLP trace export compression.
	OTEL_EXPORTER_OTLP_COMPRESSION = "OTEL_EXPORTER_OTLP_COMPRESSION"
	// OTEL_TRACES_SAMPLER configures the OpenTelemetry traces sampler.
	OTEL_TRACES_SAMPLER = "OTEL_TRACES_SAMPLER"
	// OTEL_TRACES_SAMPLER_ARG configures the OpenTelemetry traces sampler argument.
	OTEL_TRACES_SAMPLER_ARG = "OTEL_TRACES_SAMPLER_ARG"
	// OTEL_BSP_MAX_QUEUE_SIZE configures the BatchSpanProcessor queue capacity.
	OTEL_BSP_MAX_QUEUE_SIZE = "OTEL_BSP_MAX_QUEUE_SIZE"
	// OTEL_BSP_MAX_EXPORT_BATCH_SIZE configures the BatchSpanProcessor export batch limit.
	OTEL_BSP_MAX_EXPORT_BATCH_SIZE = "OTEL_BSP_MAX_EXPORT_BATCH_SIZE"
	// OTEL_BSP_SCHEDULE_DELAY configures the BatchSpanProcessor schedule delay.
	OTEL_BSP_SCHEDULE_DELAY = "OTEL_BSP_SCHEDULE_DELAY"
	// OTEL_BSP_EXPORT_TIMEOUT configures the BatchSpanProcessor export timeout.
	OTEL_BSP_EXPORT_TIMEOUT = "OTEL_BSP_EXPORT_TIMEOUT"
)

// OTLPProtocol is the transport protocol used by OTLP trace exporters.
type OTLPProtocol string

const (
	// OTLPProtocolGRPC exports traces with OTLP over gRPC.
	OTLPProtocolGRPC OTLPProtocol = "grpc"
	// OTLPProtocolHTTPProtobuf exports traces with OTLP over HTTP using protobuf payloads.
	OTLPProtocolHTTPProtobuf OTLPProtocol = "http/protobuf"
)

// UnmarshalText reads text as one of the protocols, in any letter case; any
// other text is a configuration error naming the key.
func (p *OTLPProtocol) UnmarshalText(text []byte) (err error) {
	*p, err = oneOf(text, OTLPProtocolGRPC, OTLPProtocolHTTPProtobuf)
	return err
}

// OTLPCompression is the compression mode used by OTLP trace exporters.
type OTLPCompression string

const (
	// OTLPCompressionNone sends OTLP trace payloads without compression.
	OTLPCompressionNone OTLPCompression = "none"
	// OTLPCompressionGzip compresses OTLP trace payloads with gzip.
	OTLPCompressionGzip OTLPCompression = "gzip"
)

// UnmarshalText reads text as one of the compression modes, in any letter
// case; any other text is a configuration error naming the key.
func (c *OTLPCompression) UnmarshalText(text []byte) (err error) {
	*c, err = oneOf(text, OTLPCompressionNone, OTLPCompressionGzip)
	return err
}

// TracesSampler is the OpenTelemetry sampler name used for traces.
type TracesSampler string

const (
	// TracesSamplerAlwaysOn samples every trace.
	TracesSamplerAlwaysOn TracesSampler = "always_on"
	// TracesSamplerAlwaysOff drops every trace.
	TracesSamplerAlwaysOff TracesSampler = "always_off"
	// TracesSamplerTraceIDRatio samples root traces by trace ID ratio.
	TracesSamplerTraceIDRatio TracesSampler = "traceidratio"
	// TracesSamplerParentBasedAlwaysOn samples roots and honors upstream parent decisions.
	TracesSamplerParentBasedAlwaysOn TracesSampler = "parentbased_always_on"
	// TracesSamplerParentBasedAlwaysOff drops roots and honors upstream parent decisions.
	TracesSamplerParentBasedAlwaysOff TracesSampler = "parentbased_always_off"
	// TracesSamplerParentBasedTraceIDRatio samples roots by ratio and honors upstream parent decisions.
	TracesSamplerParentBasedTraceIDRatio TracesSampler = "parentbased_traceidratio"
)

// UnmarshalText reads text as one of the sampler names, in any letter case;
// any other text is a configuration error naming the key.
func (s *TracesSampler) UnmarshalText(text []byte) (err error) {
	*s, err = oneOf(text, TracesSamplerAlwaysOn, TracesSamplerAlwaysOff, TracesSamplerTraceIDRatio,
		TracesSamplerParentBasedAlwaysOn, TracesSamplerParentBasedAlwaysOff, TracesSamplerParentBasedTraceIDRatio)
	return err
}

// OTEL represents OpenTelemetry tracing configuration using OTLP exporters.
// This configuration supports sending traces to Jaeger, Uptrace, or other OTLP-compatible backends.
type OTEL struct {
	// Enabled controls whether OpenTelemetry tracing is enabled.
	Enabled bool `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`

	// ServiceName is the OpenTelemetry service.name resource attribute.
	ServiceName string `json:"service_name" mapstructure:"service_name" ini:"service_name" yaml:"service_name"`

	// ExporterOTLPProtocol selects the OTLP transport protocol: grpc or http/protobuf.
	ExporterOTLPProtocol OTLPProtocol `json:"exporter_otlp_protocol" mapstructure:"exporter_otlp_protocol" ini:"exporter_otlp_protocol" yaml:"exporter_otlp_protocol"`

	// ExporterOTLPTracesEndpoint is the trace-specific OTLP endpoint URL.
	ExporterOTLPTracesEndpoint string `json:"exporter_otlp_traces_endpoint" mapstructure:"exporter_otlp_traces_endpoint" ini:"exporter_otlp_traces_endpoint" yaml:"exporter_otlp_traces_endpoint"`

	// ExporterOTLPHeaders are key-value headers sent with OTLP requests.
	ExporterOTLPHeaders map[string]string `json:"exporter_otlp_headers" mapstructure:"exporter_otlp_headers" ini:"exporter_otlp_headers" yaml:"exporter_otlp_headers"`

	// ExporterOTLPCompression controls OTLP payload compression: none or gzip.
	ExporterOTLPCompression OTLPCompression `json:"exporter_otlp_compression" mapstructure:"exporter_otlp_compression" ini:"exporter_otlp_compression" yaml:"exporter_otlp_compression"`

	// TracesSampler selects the OpenTelemetry trace sampler.
	TracesSampler TracesSampler `json:"traces_sampler" mapstructure:"traces_sampler" ini:"traces_sampler" yaml:"traces_sampler"`

	// TracesSamplerArg configures samplers that need an argument, such as traceidratio.
	TracesSamplerArg string `json:"traces_sampler_arg" mapstructure:"traces_sampler_arg" ini:"traces_sampler_arg" yaml:"traces_sampler_arg"`

	// BSPMaxQueueSize is the BatchSpanProcessor queue capacity.
	BSPMaxQueueSize int `json:"bsp_max_queue_size" mapstructure:"bsp_max_queue_size" ini:"bsp_max_queue_size" yaml:"bsp_max_queue_size"`

	// BSPMaxExportBatchSize is the maximum number of spans exported in one batch.
	BSPMaxExportBatchSize int `json:"bsp_max_export_batch_size" mapstructure:"bsp_max_export_batch_size" ini:"bsp_max_export_batch_size" yaml:"bsp_max_export_batch_size"`

	// BSPScheduleDelay is the maximum delay between two consecutive exports.
	BSPScheduleDelay time.Duration `json:"bsp_schedule_delay" mapstructure:"bsp_schedule_delay" ini:"bsp_schedule_delay" yaml:"bsp_schedule_delay"`

	// BSPExportTimeout is the maximum duration allowed for one export attempt.
	BSPExportTimeout time.Duration `json:"bsp_export_timeout" mapstructure:"bsp_export_timeout" ini:"bsp_export_timeout" yaml:"bsp_export_timeout"`
}

func (o *OTEL) setDefaults(v *viper.Viper) {
	v.SetDefault("otel.enabled", false)
	v.SetDefault("otel.service_name", types.FrameworkName)
	v.SetDefault("otel.exporter_otlp_protocol", OTLPProtocolHTTPProtobuf)
	v.SetDefault("otel.exporter_otlp_traces_endpoint", "http://localhost:4318/v1/traces")
	v.SetDefault("otel.exporter_otlp_headers", map[string]string{})
	v.SetDefault("otel.exporter_otlp_compression", OTLPCompressionNone)
	v.SetDefault("otel.traces_sampler", TracesSamplerParentBasedAlwaysOn)
	v.SetDefault("otel.traces_sampler_arg", "")
	v.SetDefault("otel.bsp_max_queue_size", 2048)
	v.SetDefault("otel.bsp_max_export_batch_size", 512)
	v.SetDefault("otel.bsp_schedule_delay", 5*time.Second)
	v.SetDefault("otel.bsp_export_timeout", 30*time.Second)
}
