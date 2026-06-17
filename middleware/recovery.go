package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"go.uber.org/zap"
)

func Recovery(filename string) gin.HandlerFunc {
	// TODO: replace it using custom logger.
	return ginzap.RecoveryWithZap(pkgzap.NewGin(filename), true)
}

// RecoveryWithTracing returns a gin.HandlerFunc (middleware)
// that recovers from any panics and logs requests using uber-go/zap.
// All errors are logged using zap.Error().
// stack means whether output the stack info.
// The stack info is easy to find where the error occurs but the stack info is too large.
func RecoveryWithTracing(logger *zap.Logger, stack bool) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		// Record panic in tracing span
		span := GetSpanFromContext(c)
		if span != nil && span.IsRecording() {
			RecordError(c, fmt.Errorf("panic recovered: %v", recovered))
			AddSpanTags(c, map[string]any{
				"error.panic":     true,
				"error.recovered": fmt.Sprintf("%v", recovered),
			})
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

			if brokenPipe {
				logger.Error(fmt.Sprintf("%s\n%s", recovered, headersToStr))
			} else if stack {
				logger.Error(fmt.Sprintf("[Recovery] %s panic recovered:\n%s\n%s\n%s",
					timeFormat(time.Now()), headersToStr, recovered, debug.Stack()))
			} else {
				logger.Error(fmt.Sprintf("[Recovery] %s panic recovered:\n%s\n%s",
					timeFormat(time.Now()), headersToStr, recovered))
			}
		}

		// If the connection is dead, we can't write a status to it.
		if brokenPipe {
			c.Error(recovered.(error)) //nolint: errcheck
			c.Abort()
		} else {
			c.AbortWithStatus(http.StatusInternalServerError)
		}
	})
}

func timeFormat(t time.Time) string {
	return t.Format("2006/01/02 - 15:04:05")
}
