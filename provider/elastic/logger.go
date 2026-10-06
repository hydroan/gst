package elastic

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/hydroan/gst/internal/logfield"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// elasticLogger is a simple logger adapter that uses the zap logger.
type elasticLogger struct {
	logger types.Logger
}

func (l *elasticLogger) LogRoundTrip(
	req *http.Request,
	res *http.Response,
	err error,
	start time.Time,
	dur time.Duration,
) error {
	var (
		status int
		body   string
	)

	if res != nil {
		status = res.StatusCode
		if res.Body != nil {
			bodyBytes, _ := io.ReadAll(res.Body)
			res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			body = string(bodyBytes)
		}
	}

	l.logger.Debugz(
		"Elasticsearch HTTP Request",
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		logfield.Status(status),
		util.LogDuration(dur),
		zap.Error(err),
		zap.String("response", body),
	)

	return nil
}

// RequestBodyEnabled is required for the Logger interface
func (l *elasticLogger) RequestBodyEnabled() bool {
	return true
}

// ResponseBodyEnabled is required for the Logger interface
func (l *elasticLogger) ResponseBodyEnabled() bool {
	return true
}
