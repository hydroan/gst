package config

import (
	"time"

	"github.com/hydroan/gst/types/consts"
)

const (
	// OTEL_ENABLE enables OpenTelemetry tracing.
	OTEL_ENABLE = "OTEL_ENABLE" //nolint:staticcheck
	// OTEL_SERVICE_NAME configures the OpenTelemetry service.name resource attribute.
	OTEL_SERVICE_NAME = "OTEL_SERVICE_NAME" //nolint:staticcheck
	// OTEL_EXPORTER_OTLP_PROTOCOL configures the OTLP traces transport protocol.
	OTEL_EXPORTER_OTLP_PROTOCOL = "OTEL_EXPORTER_OTLP_PROTOCOL" //nolint:staticcheck
	// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT configures the trace-specific OTLP endpoint.
	OTEL_EXPORTER_OTLP_TRACES_ENDPOINT = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT" //nolint:staticcheck
	// OTEL_EXPORTER_OTLP_HEADERS configures headers sent with OTLP trace exports.
	OTEL_EXPORTER_OTLP_HEADERS = "OTEL_EXPORTER_OTLP_HEADERS" //nolint:staticcheck
	// OTEL_EXPORTER_OTLP_COMPRESSION configures OTLP trace export compression.
	OTEL_EXPORTER_OTLP_COMPRESSION = "OTEL_EXPORTER_OTLP_COMPRESSION" //nolint:staticcheck
	// OTEL_TRACES_SAMPLER configures the OpenTelemetry traces sampler.
	OTEL_TRACES_SAMPLER = "OTEL_TRACES_SAMPLER" //nolint:staticcheck
	// OTEL_TRACES_SAMPLER_ARG configures the OpenTelemetry traces sampler argument.
	OTEL_TRACES_SAMPLER_ARG = "OTEL_TRACES_SAMPLER_ARG" //nolint:staticcheck
	// OTEL_LOG_SPANS controls whether spans are also written to the OTEL logger.
	OTEL_LOG_SPANS = "OTEL_LOG_SPANS" //nolint:staticcheck
	// OTEL_MAX_TAG_VALUE_LEN configures the maximum helper tag value length.
	OTEL_MAX_TAG_VALUE_LEN = "OTEL_MAX_TAG_VALUE_LEN" //nolint:staticcheck
	// OTEL_BSP_MAX_QUEUE_SIZE configures the BatchSpanProcessor queue capacity.
	OTEL_BSP_MAX_QUEUE_SIZE = "OTEL_BSP_MAX_QUEUE_SIZE" //nolint:staticcheck
	// OTEL_BSP_MAX_EXPORT_BATCH_SIZE configures the BatchSpanProcessor export batch limit.
	OTEL_BSP_MAX_EXPORT_BATCH_SIZE = "OTEL_BSP_MAX_EXPORT_BATCH_SIZE" //nolint:staticcheck
	// OTEL_BSP_SCHEDULE_DELAY configures the BatchSpanProcessor schedule delay.
	OTEL_BSP_SCHEDULE_DELAY = "OTEL_BSP_SCHEDULE_DELAY" //nolint:staticcheck
	// OTEL_BSP_EXPORT_TIMEOUT configures the BatchSpanProcessor export timeout.
	OTEL_BSP_EXPORT_TIMEOUT = "OTEL_BSP_EXPORT_TIMEOUT" //nolint:staticcheck
)

// OTLPProtocol is the transport protocol used by OTLP trace exporters.
type OTLPProtocol string

const (
	// OTLPProtocolGRPC exports traces with OTLP over gRPC.
	OTLPProtocolGRPC OTLPProtocol = "grpc"
	// OTLPProtocolHTTPProtobuf exports traces with OTLP over HTTP using protobuf payloads.
	OTLPProtocolHTTPProtobuf OTLPProtocol = "http/protobuf"
)

// OTLPCompression is the compression mode used by OTLP trace exporters.
type OTLPCompression string

const (
	// OTLPCompressionNone sends OTLP trace payloads without compression.
	OTLPCompressionNone OTLPCompression = "none"
	// OTLPCompressionGzip compresses OTLP trace payloads with gzip.
	OTLPCompressionGzip OTLPCompression = "gzip"
)

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

// OTEL represents OpenTelemetry tracing configuration using OTLP exporters.
// This configuration supports sending traces to Jaeger, Uptrace, or other OTLP-compatible backends.
type OTEL struct {
	// Enable controls whether OpenTelemetry tracing is enabled.
	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`

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

	// LogSpans controls whether spans are also written to the OTEL logger.
	LogSpans bool `json:"log_spans" mapstructure:"log_spans" ini:"log_spans" yaml:"log_spans"`

	// MaxTagValueLen is the maximum length of tag values added through helpers.
	MaxTagValueLen int `json:"max_tag_value_len" mapstructure:"max_tag_value_len" ini:"max_tag_value_len" yaml:"max_tag_value_len"`

	// BSPMaxQueueSize is the BatchSpanProcessor queue capacity.
	BSPMaxQueueSize int `json:"bsp_max_queue_size" mapstructure:"bsp_max_queue_size" ini:"bsp_max_queue_size" yaml:"bsp_max_queue_size"`

	// BSPMaxExportBatchSize is the maximum number of spans exported in one batch.
	BSPMaxExportBatchSize int `json:"bsp_max_export_batch_size" mapstructure:"bsp_max_export_batch_size" ini:"bsp_max_export_batch_size" yaml:"bsp_max_export_batch_size"`

	// BSPScheduleDelay is the maximum delay between two consecutive exports.
	BSPScheduleDelay time.Duration `json:"bsp_schedule_delay" mapstructure:"bsp_schedule_delay" ini:"bsp_schedule_delay" yaml:"bsp_schedule_delay"`

	// BSPExportTimeout is the maximum duration allowed for one export attempt.
	BSPExportTimeout time.Duration `json:"bsp_export_timeout" mapstructure:"bsp_export_timeout" ini:"bsp_export_timeout" yaml:"bsp_export_timeout"`
}

func (o *OTEL) setDefault() {
	cv.SetDefault("otel.enable", false)
	cv.SetDefault("otel.service_name", consts.FrameworkName)
	cv.SetDefault("otel.exporter_otlp_protocol", OTLPProtocolHTTPProtobuf)
	cv.SetDefault("otel.exporter_otlp_traces_endpoint", "http://localhost:4318/v1/traces")
	cv.SetDefault("otel.exporter_otlp_headers", map[string]string{})
	cv.SetDefault("otel.exporter_otlp_compression", OTLPCompressionNone)
	cv.SetDefault("otel.traces_sampler", TracesSamplerParentBasedAlwaysOn)
	cv.SetDefault("otel.traces_sampler_arg", "")
	cv.SetDefault("otel.log_spans", false)
	cv.SetDefault("otel.max_tag_value_len", 256)
	cv.SetDefault("otel.bsp_max_queue_size", 2048)
	cv.SetDefault("otel.bsp_max_export_batch_size", 512)
	cv.SetDefault("otel.bsp_schedule_delay", 5*time.Second)
	cv.SetDefault("otel.bsp_export_timeout", 30*time.Second)
}
