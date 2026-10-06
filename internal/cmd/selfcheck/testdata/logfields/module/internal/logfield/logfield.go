package logfield

import (
	"time"

	"go.uber.org/zap"
)

const statusKey = "status"

func Status(code int) zap.Field { return zap.Int(statusKey, code) }

func Threshold(d time.Duration) zap.Field { return zap.Duration("threshold", d) }
