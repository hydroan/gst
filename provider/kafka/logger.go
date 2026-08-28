package kafka

import (
	"github.com/hydroan/gst/types"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Logger adapts a framework component logger into the franz-go logger
// option. New attaches this provider's own logger by default; a caller that
// owns its client appends this option to route the client's lines into its
// own component file instead — later options win. The pointer is
// dereferenced on every line, so a logger installed after the client was
// built still receives the logs, and a nil entry (a process that never ran
// the logging setup, e.g. unit tests) drops the line.
func Logger(l *types.Logger) kgo.Opt {
	return kgo.WithLogger(kgoLogger{l: l})
}

// kgoLogger adapts the framework component logger to the kgo.Logger
// interface, routing franz-go internal logs into the standard logging
// pipeline.
type kgoLogger struct {
	l *types.Logger
}

// Level reports the maximum level franz-go should emit. Info keeps connection
// lifecycle events visible without the noise of per-request debug logs.
func (kgoLogger) Level() kgo.LogLevel { return kgo.LogLevelInfo }

// Log forwards a franz-go log line to the component logger.
func (k kgoLogger) Log(level kgo.LogLevel, msg string, keyvals ...any) {
	l := *k.l
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
