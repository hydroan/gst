package execctx

import (
	"encoding/binary"
	"encoding/hex"
	"math/rand/v2"
	"slices"
)

// NewTraceID returns a new W3C trace id for an execution outside any trace, a
// request that brought none, a cron round or a leader tenure: 16 random bytes
// in lower-case hex, never all zero, such as 4bf92f3577b34da6a3ce929d0e0e4736,
// the form the X-Trace-ID header carries and the propagator accepts.
func NewTraceID() string { return randomHex(16) }

// NewSpanID returns a new W3C span id, 8 random bytes in lower-case hex, never
// all zero, such as 00f067aa0ba902b7.
func NewSpanID() string { return randomHex(8) }

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
