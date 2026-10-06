package sample

import "go.uber.org/zap"

// mixed writes one key as a number and as a string.
func mixed() []zap.Field {
	return []zap.Field{
		zap.Int("size", 1),
		zap.String("size", "big"),
	}
}
