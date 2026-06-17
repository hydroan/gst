package middleware

import (
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/logger"
	prommetrics "github.com/hydroan/gst/metrics"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Logger(filename ...string) gin.HandlerFunc {
	// return ginzap.Ginzap(pkgzap.NewGinLogger(filename...), time.RFC3339, true)
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		c.Set(consts.CTX_ROUTE, path)
		c.Next()

		labelPath := sanitizeLabelValue(c.FullPath())
		prommetrics.HTTPRequestsTotal.WithLabelValues(c.Request.Method, labelPath, strconv.Itoa(c.Writer.Status())).Inc()
		prommetrics.HTTPRequestDuration.WithLabelValues(c.Request.Method, labelPath, strconv.Itoa(c.Writer.Status())).Observe(time.Since(start).Seconds())

		// Add tracing information to logs
		span := GetSpanFromContext(c)
		traceFields := []zapcore.Field{}
		if span != nil && span.IsRecording() {
			spanContext := span.SpanContext()
			if spanContext.HasTraceID() {
				traceFields = append(
					traceFields,
					zap.String("trace_id", spanContext.TraceID().String()),
					zap.String("span_id", spanContext.SpanID().String()),
				)
			}
		}

		//nolint:prealloc
		fields := []zapcore.Field{
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String(consts.CTX_USERNAME, c.GetString(consts.CTX_USERNAME)),
			zap.String(consts.CTX_USER_ID, c.GetString(consts.CTX_USER_ID)),
			zap.String(consts.REQUEST_ID, c.GetString(consts.REQUEST_ID)),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.String("latency", util.FormatDurationSmart(time.Since(start))),
		}

		// Append tracing fields to log fields
		fields = append(fields, traceFields...)

		if len(c.Errors) > 0 {
			// Append error field if this is an erroneous request.
			for _, e := range c.Errors.Errors() {
				logger.Gin.Error(e, fields...)
			}
		} else {
			logger.Gin.Info(path, fields...)
		}
	}
}

// sanitizeLabelValue ensures we never export non UTF-8 label values to Prometheus.
func sanitizeLabelValue(value string) string {
	if value == "" {
		return "<empty>"
	}

	if utf8.ValidString(value) {
		return value
	}

	return "<invalid>"
}
