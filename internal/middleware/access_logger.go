package middleware

import (
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/logger"
	prommetrics "github.com/hydroan/gst/metrics"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// accessLogFieldCap is the most fields one access-log entry carries: the
// eleven every request has — status, method, username, user id, trace id,
// route, path, query, ip, user agent and the duration, whose two keys
// util.LogDuration renders from one inlined field — plus the span id of a
// request a recording span traces, plus the error of a request that
// reported one. The logger runs on every request and sizes its field slice
// to it once, so the hot path never regrows it; a field added to
// accessLogger bumps it, which the worst-case test enforces.
const accessLogFieldCap = 13

func accessLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		c.Next()

		route := c.FullPath()
		labelRoute := sanitizeLabelValue(route)
		prommetrics.HTTPRequestsTotal.WithLabelValues(c.Request.Method, labelRoute, strconv.Itoa(c.Writer.Status())).Inc()
		prommetrics.HTTPRequestDuration.WithLabelValues(c.Request.Method, labelRoute, strconv.Itoa(c.Writer.Status())).Observe(time.Since(start).Seconds())

		// Add tracing information to logs.
		span := requestSpan(c)
		traceID := c.GetString(consts.TRACE_ID)
		var spanID string
		if span != nil && span.IsRecording() {
			spanContext := span.SpanContext()
			if spanContext.HasTraceID() {
				if traceID == "" {
					traceID = spanContext.TraceID().String()
				}
				spanID = spanContext.SpanID().String()
			}
		}

		fields := make([]zapcore.Field, 0, accessLogFieldCap)
		fields = append(
			fields,
			zap.Int("status", c.Writer.Status()),
			zap.String(consts.CTX_METHOD, c.Request.Method),
			zap.String(consts.CTX_USERNAME, c.GetString(consts.CTX_USERNAME)),
			zap.String(consts.CTX_USER_ID, c.GetString(consts.CTX_USER_ID)),
			zap.String(consts.TRACE_ID, traceID),
			zap.String(consts.CTX_ROUTE, route),
			zap.String(consts.CTX_PATH, path),
			zap.String(consts.QUERY, query),
			zap.String("ip", requestctx.GinClientIP(c)),
			zap.String("user_agent", c.Request.UserAgent()),
			util.LogDuration(time.Since(start)),
		)
		if spanID != "" {
			fields = append(fields, zap.String("span_id", spanID))
		}

		if len(c.Errors) > 0 {
			// A request that reported errors logs one entry per error, the
			// error in a field of its own, like the gRPC access log's: the
			// encoder of logger.Gin drops the message (see logger.NewGin).
			// The field takes the last slot of the capacity, so each entry
			// overwrites the previous one's.
			for _, e := range c.Errors.Errors() {
				logger.Gin.Error(e, append(fields, zap.String("error", e))...)
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
