package elastic

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/hydroan/gst/types"
)

// A simple logger adapter that uses zap logger
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

	l.logger.Debugw(
		"Elasticsearch HTTP Request",
		"method", req.Method,
		"url", req.URL.String(),
		"status", status,
		"duration", dur,
		"error", err,
		"response", body,
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
