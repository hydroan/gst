package util

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type logDuration time.Duration

func (d logDuration) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt64("duration", int64(d))
	return nil
}

func LogDuration(d time.Duration) zap.Field { return zap.Inline(logDuration(d)) }
