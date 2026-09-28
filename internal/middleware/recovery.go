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
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// recovery returns the recovery middleware the default router chain installs,
// logging panics to logger.Recovery, the recovery log the gRPC listener's
// panics go to as well.
//
// It binds recoveryWithTracing to that logger rather than standing a second
// implementation beside it: two implementations drift apart, and a drifted one
// leaves the request's span with no error recorded on it, logs the
// Authorization header as it stands, or answers with a bare 500 carrying no
// envelope and no trace id — on the one response whose reader most needs one.
func recovery() gin.HandlerFunc {
	return recoveryWithTracing(logger.Recovery, true)
}

// recoveryWithTracing returns a gin.HandlerFunc (middleware)
// that recovers from any panics and logs requests using uber-go/zap.
// All errors are logged using zap.Error().
// stack means whether output the stack info.
// The stack info is easy to find where the error occurs but the stack info is too large.
func recoveryWithTracing(log *zap.Logger, stack bool) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		// Record panic in tracing span
		span := requestSpan(c)
		if span != nil && span.IsRecording() {
			gstotel.RecordError(span, fmt.Errorf("panic recovered: %v", recovered))
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

		if log != nil {
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
			// the message would be zone-less text on the host clock. The trace
			// id is a field, so one search of the file finds the panic that
			// explains a response.
			traceID := zap.String(consts.TRACE_ID, c.GetString(consts.TRACE_ID))
			switch {
			case brokenPipe:
				log.Error(fmt.Sprintf("%s\n%s", recovered, headersToStr), traceID)
			case stack:
				log.Error(fmt.Sprintf("[recovery] panic recovered:\n%s\n%s\n%s",
					headersToStr, recovered, debug.Stack()), traceID)
			default:
				log.Error(fmt.Sprintf("[recovery] panic recovered:\n%s\n%s",
					headersToStr, recovered), traceID)
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
			response.Abort(c, http.StatusInternalServerError, serviceregistry.FailureMsg)
		}
	})
}
