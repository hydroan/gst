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
// installed by the logging setup (a global-sink fallback, replaced with the
// dedicated kafka logger by bootstrap's provider drain); in a process that
// never ran that setup (e.g. unit tests) it is still nil and the line is
// dropped.
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
