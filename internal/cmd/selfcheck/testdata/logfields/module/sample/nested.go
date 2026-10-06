package sample

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type extra struct{}

func (extra) MarshalLogObject(zapcore.ObjectEncoder) error { return nil }

// nested nests keys under the entry outside the packages allowed to.
func nested() zap.Field { return zap.Object("extra", extra{}) }

// weird calls a constructor the check does not classify.
func weird() zap.Field { return zap.Weird("odd", 1) }
