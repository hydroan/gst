// Package zap stands in for go.uber.org/zap in the fixture: the check reads
// the constructors by package path and name, so the stand-in declares the
// names the fixture calls and nothing of what they do. Weird is a
// constructor the check does not classify.
package zap

import (
	"time"

	"go.uber.org/zap/zapcore"
)

type Field = zapcore.Field

func String(key, val string) Field                         { return Field{Key: key} }
func Int(key string, val int) Field                        { return Field{Key: key} }
func Duration(key string, val time.Duration) Field         { return Field{Key: key} }
func Any(key string, val any) Field                        { return Field{Key: key} }
func Reflect(key string, val any) Field                    { return Field{Key: key} }
func Object(key string, val zapcore.ObjectMarshaler) Field { return Field{Key: key} }
func Inline(val zapcore.ObjectMarshaler) Field             { return Field{} }
func Weird(key string, val int) Field                      { return Field{Key: key} }
