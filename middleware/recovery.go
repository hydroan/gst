package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/response"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// recovery returns the recovery middleware the default router chain installs,
// logging panics to filename.
//
// It binds recoveryWithTracing to a file logger rather than standing a second
// implementation beside it. The two had drifted: a panic handled here left the
// request's span with no error recorded on it, logged the Authorization header
// as it stood, and answered with a bare 500 carrying no envelope and no trace
// id — on the one response whose reader most needs one.
func recovery(filename string) gin.HandlerFunc {
	return recoveryWithTracing(pkgzap.NewGin(filename), true)
}

// recoveryWithTracing returns a gin.HandlerFunc (middleware)
// that recovers from any panics and logs requests using uber-go/zap.
// All errors are logged using zap.Error().
// stack means whether output the stack info.
// The stack info is easy to find where the error occurs but the stack info is too large.
func recoveryWithTracing(logger *zap.Logger, stack bool) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		// Record panic in tracing span
		span := GetSpanFromContext(c)
		if span != nil && span.IsRecording() {
			RecordError(c, fmt.Errorf("panic recovered: %v", recovered))
			span.SetAttributes(
				attribute.Bool("error.panic", true),
				attribute.String("error.recovered", fmt.Sprintf("%v", recovered)),
			)
		}

		// Check for a broken connection, as it is not really a
		// condition that warrants a panic stack trace.
		var brokenPipe bool
		if ne, ok := recovered.(*net.OpError); ok {
			var se *os.SyscallError
			if errors.As(ne, &se) {
				seStr := strings.ToLower(se.Error())
				if strings.Contains(seStr, "broken pipe") ||
					strings.Contains(seStr, "connection reset by peer") {
					brokenPipe = true
				}
			}
		}

		if logger != nil {
			httpRequest, _ := httputil.DumpRequest(c.Request, false)
			headers := strings.Split(string(httpRequest), "\r\n")
			for idx, header := range headers {
				current := strings.Split(header, ":")
				if current[0] == "Authorization" {
					headers[idx] = current[0] + ": *"
				}
			}
			headersToStr := strings.Join(headers, "\r\n")

			// The entry timestamp is the encoder's job; a hand-rolled one in
			// the message would be zone-less text on the host clock.
			switch {
			case brokenPipe:
				logger.Error(fmt.Sprintf("%s\n%s", recovered, headersToStr))
			case stack:
				logger.Error(fmt.Sprintf("[recovery] panic recovered:\n%s\n%s\n%s",
					headersToStr, recovered, debug.Stack()))
			default:
				logger.Error(fmt.Sprintf("[recovery] panic recovered:\n%s\n%s",
					headersToStr, recovered))
			}
		}

		// If the connection is dead, we can't write a status to it.
		if brokenPipe {
			c.Error(recovered.(error)) //nolint: errcheck
			c.Abort()
		} else {
			// What the panic was stays in the log above. The caller gets the
			// envelope every other response carries, so one reader can parse
			// them all and quote back the trace id that explains this one.
			response.Abort(c, http.StatusInternalServerError, "internal server error")
		}
	})
}
