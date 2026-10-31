package sample

import (
	"time"

	"go.uber.org/zap"
)

// slices writes keys holding slices: tags is an array read by zap.Any and a
// string elsewhere, which the check reports; blob, a slice of bytes, is a
// string like the string written beside it; rows, a reflected slice, is the
// string the encoder collapses it into, like the string beside it; and the
// slices of durations and times are arrays written once.
func slices() []zap.Field {
	return []zap.Field{
		zap.Any("tags", []string{"a"}),
		zap.String("tags", "a"),
		zap.Any("blob", []byte("a")),
		zap.String("blob", "a"),
		zap.Reflect("rows", []int{1}),
		zap.String("rows", "1"),
		zap.Any("waits", []time.Duration{time.Second}),
		zap.Any("stamps", []time.Time{{}}),
	}
}
