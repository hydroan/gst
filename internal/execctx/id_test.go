package execctx_test

import (
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/execctx"
	"github.com/stretchr/testify/require"
)

// TestNewTraceIDAndNewSpanIDAreW3CIDs pins the form of the ids the framework
// generates for an execution outside any trace: a trace id of 16 random
// bytes and a span id of 8, in lower-case hex and never all zero, the form
// the W3C trace context defines and the X-Trace-ID header carries.
func TestNewTraceIDAndNewSpanIDAreW3CIDs(t *testing.T) {
	first, second := execctx.NewTraceID(), execctx.NewTraceID()
	require.Regexp(t, `^[0-9a-f]{32}$`, first)
	require.NotEqual(t, strings.Repeat("0", 32), first)
	require.NotEqual(t, first, second)

	span, other := execctx.NewSpanID(), execctx.NewSpanID()
	require.Regexp(t, `^[0-9a-f]{16}$`, span)
	require.NotEqual(t, strings.Repeat("0", 16), span)
	require.NotEqual(t, span, other)
}

// TestTraceIDOfKeepsAW3CTraceIDAndReplacesTheRest pins the one rule both
// listeners read the X-Trace-ID of a request by, tracing on or off: a W3C
// trace id, 32 lower-case hex digits that are not all zero, is kept as sent,
// surrounding space aside; anything else, a bare word, upper-case hex, a
// zero id, an empty header or a long string, is replaced by a new id of the
// same form, so text that is not an id never reaches a log, a statement
// comment or a response header.
func TestTraceIDOfKeepsAW3CTraceIDAndReplacesTheRest(t *testing.T) {
	const sent = "4bf92f3577b34da6a3ce929d0e0e4736"
	require.Equal(t, sent, execctx.TraceIDOf(sent))
	require.Equal(t, sent, execctx.TraceIDOf(" "+sent+"\t"), "surrounding space is the writer's, not part of the id")

	for name, candidate := range map[string]string{
		"a bare word":   "trace-1",
		"upper case":    strings.ToUpper(sent),
		"all zero":      strings.Repeat("0", 32),
		"too short":     sent[:31],
		"empty":         "",
		"a long string": strings.Repeat("a", 4096),
	} {
		got := execctx.TraceIDOf(candidate)
		require.Regexp(t, `^[0-9a-f]{32}$`, got, name)
		require.NotEqual(t, candidate, got, name)
		require.NotEqual(t, strings.Repeat("0", 32), got, name)
	}
}

func BenchmarkNewTraceID(b *testing.B) {
	for b.Loop() {
		execctx.NewTraceID()
	}
}
