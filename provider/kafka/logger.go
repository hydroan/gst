package kafka

import (
	"github.com/hydroan/gst/logger"
	"github.com/twmb/franz-go/pkg/kgo"
)

// kgoLogger adapts the framework component logger to the kgo.Logger interface,
// routing franz-go internal logs into the standard logging pipeline.
type kgoLogger struct{}

// Level reports the maximum level franz-go should emit. Info keeps connection
// lifecycle events visible without the noise of per-request debug logs.
func (kgoLogger) Level() kgo.LogLevel { return kgo.LogLevelInfo }

// Log forwards a franz-go log line to the component logger. logger.Kafka is
// wired during bootstrap; outside a bootstrapped process (e.g. unit tests)
// the line is dropped.
func (kgoLogger) Log(level kgo.LogLevel, msg string, keyvals ...any) {
	l := logger.Kafka
	if l == nil {
		return
	}
	switch level {
	case kgo.LogLevelError:
		l.Errorw(msg, keyvals...)
	case kgo.LogLevelWarn:
		l.Warnw(msg, keyvals...)
	case kgo.LogLevelInfo:
		l.Infow(msg, keyvals...)
	default:
		l.Debugw(msg, keyvals...)
	}
}
