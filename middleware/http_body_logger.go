package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

const defaultHTTPBodyLogMaxSize = "64KB"

// BodyLogger returns a middleware that logs JSON HTTP request and response bodies.
func BodyLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.App.Logger.HTTPBody
		if !cfg.Enabled || (!cfg.LogRequest && !cfg.LogResponse) {
			c.Next()
			return
		}

		maxBodySize := httpBodyLogMaxSize(cfg.MaxBodySize)

		var reqLog *bodyLogEntry
		if cfg.LogRequest {
			reqLog = captureRequestBody(c, maxBodySize)
		}

		var writer *bodyLogWriter
		if cfg.LogResponse {
			writer = newBodyLogWriter(c.Writer, maxBodySize)
			c.Writer = writer
		}

		c.Next()

		if cfg.LogRequest && reqLog != nil {
			writeHTTPBodyLog(c, reqLog, false)
		}
		if cfg.LogResponse && writer != nil {
			if rspLog := captureResponseBody(c, writer, maxBodySize); rspLog != nil {
				writeHTTPBodyLog(c, rspLog, true)
			}
		}
	}
}

type bodyLogEntry struct {
	msg   string
	key   string
	value any
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body  bytes.Buffer
	limit int64
}

func newBodyLogWriter(writer gin.ResponseWriter, maxBodySize int64) *bodyLogWriter {
	return &bodyLogWriter{
		ResponseWriter: writer,
		limit:          maxBodySize + 1,
	}
}

func (w *bodyLogWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

func (w *bodyLogWriter) WriteString(data string) (int, error) {
	w.capture([]byte(data))
	return w.ResponseWriter.WriteString(data)
}

func (w *bodyLogWriter) capture(data []byte) {
	remaining := w.limit - int64(w.body.Len())
	if remaining <= 0 {
		return
	}
	if int64(len(data)) > remaining {
		data = data[:remaining]
	}
	_, _ = w.body.Write(data)
}

func captureRequestBody(c *gin.Context, maxBodySize int64) *bodyLogEntry {
	contentType := c.GetHeader("Content-Type")
	if !isJSONContentType(contentType) {
		return nil
	}

	switch {
	case c.Request.ContentLength > maxBodySize:
		return &bodyLogEntry{msg: "request body too large"}
	case c.Request.ContentLength < 0:
		return &bodyLogEntry{msg: "request body size unknown"}
	case c.Request.Body == nil:
		return &bodyLogEntry{msg: "request", key: "request", value: map[string]any{}}
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return &bodyLogEntry{msg: "request body read failed"}
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	if len(body) == 0 {
		return &bodyLogEntry{msg: "request", key: "request", value: map[string]any{}}
	}

	data, ok := unmarshalJSONBody(body)
	if !ok {
		return &bodyLogEntry{msg: "request body invalid json"}
	}

	return &bodyLogEntry{msg: "request", key: "request", value: data}
}

func captureResponseBody(c *gin.Context, writer *bodyLogWriter, maxBodySize int64) *bodyLogEntry {
	contentType := c.Writer.Header().Get("Content-Type")
	if !isJSONContentType(contentType) {
		return nil
	}

	body := writer.body.Bytes()
	if int64(len(body)) > maxBodySize {
		return &bodyLogEntry{msg: "response body too large"}
	}
	if len(body) == 0 {
		return &bodyLogEntry{msg: "response", key: "response", value: map[string]any{}}
	}

	data, ok := unmarshalJSONBody(body)
	if !ok {
		return &bodyLogEntry{msg: "response body invalid json"}
	}

	return &bodyLogEntry{msg: "response", key: "response", value: data}
}

func writeHTTPBodyLog(c *gin.Context, entry *bodyLogEntry, response bool) {
	if logger.Gin == nil || entry == nil {
		return
	}

	fields := httpBodyLogFields(c, response)
	if entry.key != "" {
		fields = append(fields, zap.Any(entry.key, entry.value))
	}

	logger.Gin.Info(entry.msg, fields...)
}

func httpBodyLogFields(c *gin.Context, response bool) []zap.Field {
	fields := []zap.Field{
		zap.String("route", httpBodyLogRoute(c)),
		zap.String("method", c.Request.Method),
		zap.String(consts.CTX_USERNAME, c.GetString(consts.CTX_USERNAME)),
		zap.String(consts.CTX_USER_ID, c.GetString(consts.CTX_USER_ID)),
		zap.String(consts.TRACE_ID, c.GetString(consts.TRACE_ID)),
		zap.Any(consts.PARAMS, httpBodyLogParams(c.Params)),
		zap.Any(consts.QUERY, c.Request.URL.Query()),
	}

	if response {
		fields = append(
			fields,
			zap.Int("status", c.Writer.Status()),
			zap.String("content_type", c.Writer.Header().Get("Content-Type")),
			zap.Int("content_length", c.Writer.Size()),
		)
	} else {
		fields = append(
			fields,
			zap.String("content_type", c.GetHeader("Content-Type")),
			zap.Int64("content_length", c.Request.ContentLength),
		)
	}

	return fields
}

func httpBodyLogRoute(c *gin.Context) string {
	if route := c.GetString(consts.CTX_ROUTE); route != "" {
		return route
	}
	if route := c.FullPath(); route != "" {
		return route
	}
	return c.Request.URL.Path
}

func httpBodyLogParams(params gin.Params) map[string]string {
	values := make(map[string]string, len(params))
	for _, param := range params {
		values[param.Key] = param.Value
	}
	return values
}

func httpBodyLogMaxSize(value string) int64 {
	size, err := humanize.ParseBytes(value)
	if err == nil && size > 0 {
		return safeHTTPBodyLogSize(size)
	}

	size, err = humanize.ParseBytes(defaultHTTPBodyLogMaxSize)
	if err == nil && size > 0 {
		return safeHTTPBodyLogSize(size)
	}
	return 64 * 1024
}

func safeHTTPBodyLogSize(size uint64) int64 {
	const maxInt64AsUint64 = uint64(1<<63 - 1)
	if size > maxInt64AsUint64 {
		return 1<<63 - 1
	}
	return int64(size) // #nosec G115 -- size is capped to MaxInt64 before conversion.
}

func isJSONContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func unmarshalJSONBody(body []byte) (any, bool) {
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, false
	}
	return data, true
}

var _ http.ResponseWriter = (*bodyLogWriter)(nil)
