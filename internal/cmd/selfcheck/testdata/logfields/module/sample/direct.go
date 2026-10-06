package sample

import "go.uber.org/zap"

// direct writes keys of internal/logfield without it, once through a
// constructor and once as a key-value pair.
func direct() zap.Field {
	log.Infow("slow", "threshold", "200ms")
	return zap.String("status", "OK")
}
