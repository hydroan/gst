package execctx

import (
	"encoding/binary"
	"encoding/hex"
	"math/rand/v2"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// NewTraceID returns a new W3C trace id for an execution outside any trace, a
// request that brought none, a cron round or a leader tenure: 16 random bytes
// in lower-case hex, never all zero, such as 4bf92f3577b34da6a3ce929d0e0e4736,
// the form the X-Trace-ID header carries and the propagator accepts.
func NewTraceID() string { return randomHex(16) }

// NewSpanID returns a new W3C span id, 8 random bytes in lower-case hex, never
// all zero, such as 00f067aa0ba902b7.
func NewSpanID() string { return randomHex(8) }

// TraceIDOf returns the trace id a request carries in candidate, the value of
// its X-Trace-ID header or metadata, when it is a W3C trace id, 32 lower-case
// hex digits that are not all zero, surrounding space aside; and a new id of
// the same form otherwise. It is the one rule both listeners read the header
// by when tracing is off; with tracing on the propagator applies the same
// parse, so a request carries the same kind of id whichever way it was read,
// and text that is not an id never reaches a log, a statement comment or a
// response header.
func TraceIDOf(candidate string) string {
	if id, err := trace.TraceIDFromHex(strings.TrimSpace(candidate)); err == nil {
		return id.String()
	}
	return NewTraceID()
}

// randomHex returns n random bytes, n a multiple of 8, in lower-case hex,
// drawing again on the all-zero value the W3C trace context treats as
// absent. The ids are correlation keys, not secrets, so they come from the
// math/rand/v2 generator, the source the OpenTelemetry SDK's default id
// generator draws from, rather than the slower crypto/rand.
func randomHex(n int) string {
	b := make([]byte, n)
	for {
		for i := 0; i < n; i += 8 {
			binary.LittleEndian.PutUint64(b[i:], rand.Uint64()) //nolint:gosec // see the doc comment: not a secret
		}
		if slices.ContainsFunc(b, func(x byte) bool { return x != 0 }) {
			return hex.EncodeToString(b)
		}
	}
}
