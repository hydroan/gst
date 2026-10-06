package sample

import "go.uber.org/zap"

// observer writes a key of internal/logfield from a module source, which a
// project cannot route through logfield; written with the key's type, the
// check leaves it alone.
func observer() zap.Field { return zap.Int("status", 200) }
