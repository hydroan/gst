package middleware

import (
	"bytes"
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

const (
	// httpBodyLogMessage is the log message shared by every body log entry so
	// entries can be filtered apart from access logs in the same log file.
	httpBodyLogMessage = "http_body"

	defaultHTTPBodyLogMaxSize = "64KB"
)

// BodyLogger returns a middleware that logs JSON HTTP request and response
// bodies as a single log entry per request, correlated by trace id.
//
// Bodies are logged verbatim as raw JSON text instead of being parsed, so
// malformed payloads stay visible and the log storage does not grow field
// mappings out of request content. Capturing happens up front, but whether
// the captured bodies are written is decided after the handler chain
// finished, so the all|error|none modes can use the final response status
// and the envelope code recorded by the response helpers.
func BodyLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.App.Logger.HTTPBody
		reqMode := normalizeHTTPBodyLogMode(cfg.LogRequest, config.HTTPBodyLogModeAll)
		rspMode := normalizeHTTPBodyLogMode(cfg.LogResponse, config.HTTPBodyLogModeError)
		if !cfg.Enabled ||
			(reqMode == config.HTTPBodyLogModeNone && rspMode == config.HTTPBodyLogModeNone) ||
			matchHTTPBodyLogSkipRoute(cfg.SkipRoutes, httpBodyLogRoute(c)) {
			c.Next()
			return
		}

		maxBodySize := httpBodyLogMaxSize(cfg.MaxBodySize)

		var request *httpBodyCapture
		if reqMode != config.HTTPBodyLogModeNone {
			request = captureRequestBody(c, maxBodySize)
		}

		var writer *bodyLogWriter
		if rspMode != config.HTTPBodyLogModeNone {
			writer = newBodyLogWriter(c.Writer, maxBodySize)
			c.Writer = writer
		}

		c.Next()

		var response *httpBodyCapture
		if writer != nil {
			response = captureResponseBody(c, writer, maxBodySize)
		}
		writeHTTPBodyLog(c, reqMode, rspMode, request, response)
	}
}

// httpBodyCapture holds one captured HTTP body and how complete the capture is.
type httpBodyCapture struct {
	content   []byte // raw body text; nil when no content was captured
	size      int64  // original body size in bytes, -1 when unknown
	truncated bool   // whether content was dropped or cut at the size cap
	err       string // non-empty when reading the request body failed
}

// hasContent reports whether the capture carries anything worth logging.
func (b *httpBodyCapture) hasContent() bool {
	return b != nil && (len(b.content) > 0 || b.truncated || b.err != "")
}

// captureRequestBody buffers a JSON request body and restores it for the
// handler chain. Bodies over the size cap (or of unknown length) are not read
// at all: only their size is recorded and the handler keeps the original
// stream untouched.
func captureRequestBody(c *gin.Context, maxBodySize int64) *httpBodyCapture {
	if !isJSONContentType(c.GetHeader("Content-Type")) || c.Request.Body == nil {
		return nil
	}

	switch {
	case c.Request.ContentLength > maxBodySize:
		return &httpBodyCapture{size: c.Request.ContentLength, truncated: true}
	case c.Request.ContentLength < 0:
		return &httpBodyCapture{size: -1, truncated: true}
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return &httpBodyCapture{size: c.Request.ContentLength, err: "read request body: " + err.Error()}
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	return &httpBodyCapture{content: body, size: int64(len(body))}
}

// captureResponseBody extracts the JSON response text collected by the tee
// writer. Bodies over the size cap keep a truncated prefix so oversized
// payloads remain inspectable.
func captureResponseBody(c *gin.Context, writer *bodyLogWriter, maxBodySize int64) *httpBodyCapture {
	if !isJSONContentType(c.Writer.Header().Get("Content-Type")) {
		return nil
	}

	body := writer.body.Bytes()
	if len(body) == 0 {
		return nil
	}
	size := int64(c.Writer.Size())
	if int64(len(body)) > maxBodySize {
		return &httpBodyCapture{content: body[:maxBodySize], size: size, truncated: true}
	}

	return &httpBodyCapture{content: body, size: size}
}

// bodyLogWriter tees everything written to the response into a bounded buffer
// so the body can be logged after the handler chain finished.
type bodyLogWriter struct {
	gin.ResponseWriter
	body  bytes.Buffer
	limit int64
}

var _ http.ResponseWriter = (*bodyLogWriter)(nil)

func newBodyLogWriter(writer gin.ResponseWriter, maxBodySize int64) *bodyLogWriter {
	// One byte over the cap is kept so captureResponseBody can tell a body
	// that exactly fills the cap apart from one that overflows it.
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

// writeHTTPBodyLog writes at most one log entry for the finished request,
// carrying whichever captured bodies the configured modes admit. Requests
// where neither side has content to log produce no entry at all.
func writeHTTPBodyLog(c *gin.Context, reqMode, rspMode config.HTTPBodyLogMode, request, response *httpBodyCapture) {
	if logger.Gin == nil {
		return
	}

	isError := httpBodyLogIsError(c)
	if !httpBodyLogWanted(reqMode, isError) {
		request = nil
	}
	if !httpBodyLogWanted(rspMode, isError) {
		response = nil
	}
	if !request.hasContent() && !response.hasContent() {
		return
	}

	fields := make([]zap.Field, 0, 16)
	fields = append(
		fields,
		zap.String(consts.CTX_ROUTE, httpBodyLogRoute(c)),
		zap.String("method", c.Request.Method),
		zap.String(consts.CTX_USERNAME, c.GetString(consts.CTX_USERNAME)),
		zap.String(consts.CTX_USER_ID, c.GetString(consts.CTX_USER_ID)),
		zap.String(consts.TRACE_ID, c.GetString(consts.TRACE_ID)),
		zap.Any(consts.PARAMS, httpBodyLogParams(c.Params)),
		zap.Any(consts.QUERY, c.Request.URL.Query()),
		zap.Int("status", c.Writer.Status()),
		zap.Int("code", c.GetInt(consts.CTX_RESPONSE_CODE)),
	)
	fields = appendHTTPBodyLogFields(fields, "request", request)
	fields = appendHTTPBodyLogFields(fields, "response", response)

	logger.Gin.Info(httpBodyLogMessage, fields...)
}

// appendHTTPBodyLogFields appends the body fields of one side using the side
// name ("request" or "response") as the field name prefix.
func appendHTTPBodyLogFields(fields []zap.Field, side string, body *httpBodyCapture) []zap.Field {
	if !body.hasContent() {
		return fields
	}
	if len(body.content) > 0 {
		fields = append(fields, zap.ByteString(side, body.content))
	}
	if body.size >= 0 {
		fields = append(fields, zap.Int64(side+"_size", body.size))
	}
	if body.truncated {
		fields = append(fields, zap.Bool(side+"_truncated", true))
	}
	if body.err != "" {
		fields = append(fields, zap.String(side+"_error", body.err))
	}
	return fields
}

// httpBodyLogIsError reports whether the finished request counts as failed
// for the "error" mode: an HTTP error status, or a non-zero envelope code
// recorded by the response helpers (covers coders mapped to 2xx statuses).
func httpBodyLogIsError(c *gin.Context) bool {
	return c.Writer.Status() >= http.StatusBadRequest || c.GetInt(consts.CTX_RESPONSE_CODE) != 0
}

// httpBodyLogWanted reports whether a mode admits logging for the request outcome.
func httpBodyLogWanted(mode config.HTTPBodyLogMode, isError bool) bool {
	switch mode {
	case config.HTTPBodyLogModeAll:
		return true
	case config.HTTPBodyLogModeError:
		return isError
	default:
		return false
	}
}

// normalizeHTTPBodyLogMode maps a configured mode onto a known value, falling
// back to the side's default for empty or unknown input.
func normalizeHTTPBodyLogMode(mode, fallback config.HTTPBodyLogMode) config.HTTPBodyLogMode {
	switch config.HTTPBodyLogMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case config.HTTPBodyLogModeAll:
		return config.HTTPBodyLogModeAll
	case config.HTTPBodyLogModeError:
		return config.HTTPBodyLogModeError
	case config.HTTPBodyLogModeNone:
		return config.HTTPBodyLogModeNone
	default:
		return fallback
	}
}

// matchHTTPBodyLogSkipRoute reports whether route matches a configured skip
// pattern. Patterns ending in "*" match the route by prefix; any other
// pattern must match the route exactly.
func matchHTTPBodyLogSkipRoute(patterns []string, route string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
			if strings.HasPrefix(route, prefix) {
				return true
			}
			continue
		}
		if route == pattern {
			return true
		}
	}
	return false
}

// httpBodyLogRoute resolves the route identity used for both skip matching
// and the route log field, preferring the path recorded by the access logger.
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
