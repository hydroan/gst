package logfield

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type duration time.Duration

func (d duration) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt64("duration", int64(d))
	return nil
}

func Duration(d time.Duration) zap.Field { return zap.Inline(duration(d)) }
