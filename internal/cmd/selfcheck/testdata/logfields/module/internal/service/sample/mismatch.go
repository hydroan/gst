package sample

import "go.uber.org/zap"

// mismatch writes a key of internal/logfield as another type, which the
// type rule reports for a module source as for any package.
func mismatch() zap.Field { return zap.String("status", "ok") }
