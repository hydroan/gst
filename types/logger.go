package types

import (
	itypes "github.com/hydroan/gst/internal/types"
)

// StandardLogger provides plain and printf-style leveled logging methods.
// Fatal and Fatalf follow the underlying logger's fatal behavior and should
// terminate the process after writing the log entry.
type StandardLogger = itypes.StandardLogger

// StructuredLogger provides sugared structured logging with alternating
// key/value fields. Methods with suffix "w" mean "with fields".
type StructuredLogger = itypes.StructuredLogger

// ZapLogger provides structured logging with typed zap.Field values.
// Methods with suffix "z" are the low-allocation typed-field variants.
type ZapLogger = itypes.ZapLogger

// Logger combines plain, sugared structured, and typed zap logging methods.
// With attaches string key/value fields; WithContext derives a logger carrying
// request metadata fields.
type Logger = itypes.Logger
