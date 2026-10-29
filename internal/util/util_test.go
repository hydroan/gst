package util_test

import (
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/hydroan/gst/internal/util"
	"github.com/stretchr/testify/require"
)

// TestBytesToStringConvertsWithoutCopying pins that BytesToString gives back
// the text of the bytes it is given, sharing their memory rather than
// copying it, and the empty string for nil.
func TestBytesToStringConvertsWithoutCopying(t *testing.T) {
	data := []byte("shared")
	text := util.BytesToString(data)
	require.Equal(t, "shared", text)
	require.Same(t, &data[0], unsafe.StringData(text))
	require.Empty(t, util.BytesToString(nil))
}

// TestTraceIDAndSpanIDAreW3CIDs pins the form of the ids the framework
// generates for an execution outside any trace: a trace id of 16 random
// bytes and a span id of 8, in lower-case hex and never all zero, the form
// the W3C trace context defines and the X-Trace-ID header carries.
func TestTraceIDAndSpanIDAreW3CIDs(t *testing.T) {
	first, second := util.TraceID(), util.TraceID()
	require.Regexp(t, `^[0-9a-f]{32}$`, first)
	require.NotEqual(t, strings.Repeat("0", 32), first)
	require.NotEqual(t, first, second)

	span, other := util.SpanID(), util.SpanID()
	require.Regexp(t, `^[0-9a-f]{16}$`, span)
	require.NotEqual(t, strings.Repeat("0", 16), span)
	require.NotEqual(t, span, other)
}

func BenchmarkTraceID(b *testing.B) {
	for b.Loop() {
		util.TraceID()
	}
}

func TestFormatDurationSmart(t *testing.T) {
	tests := []struct {
		input     time.Duration
		precision int
		expected  string
	}{
		// nanosecond tier: prints an integer, precision does not apply
		{0, 2, "0ns"},
		{1 * time.Nanosecond, 2, "1ns"},
		{900 * time.Nanosecond, 3, "900ns"},
		{-900 * time.Nanosecond, 3, "-900ns"},
		// tier boundaries: each unit starts exactly at 1 of that unit
		{999 * time.Nanosecond, 2, "999ns"},
		{1 * time.Microsecond, 2, "1.00µs"},
		{999500 * time.Nanosecond, 1, "999.5µs"},
		{1 * time.Millisecond, 2, "1.00ms"},
		{999 * time.Millisecond, 1, "999.0ms"},
		{1 * time.Second, 2, "1.00s"},
		{59 * time.Second, 0, "59s"},
		{60 * time.Second, 1, "1.0min"},
		// microsecond tier
		{18400 * time.Nanosecond, 2, "18.40µs"},
		{500 * time.Microsecond, 2, "500.00µs"},
		{-500 * time.Microsecond, 2, "-500.00µs"},
		// remaining tiers
		{2 * time.Millisecond, 0, "2ms"},
		{1500 * time.Millisecond, 2, "1.50s"},
		{2 * time.Second, 2, "2.00s"},
		{90 * time.Second, 3, "1.500min"},
		{2*time.Minute + 3*time.Second, 2, "2.05min"},
		{-2 * time.Second, 2, "-2.00s"},
		{-90 * time.Second, 1, "-1.5min"},
		// test precision bounds
		{123456789 * time.Nanosecond, -5, "123ms"},
		{123456789 * time.Nanosecond, 15, "123.456789000ms"},
	}

	for _, tc := range tests {
		got := util.FormatDurationSmart(tc.input, tc.precision)
		if got != tc.expected {
			t.Errorf("FormatDurationSmart(%v, %d) = %v; want %v", tc.input, tc.precision, got, tc.expected)
		}
	}

	// precision defaults to 2 when omitted
	if got := util.FormatDurationSmart(2120 * time.Microsecond); got != "2.12ms" {
		t.Errorf("FormatDurationSmart(2120µs) = %v; want 2.12ms", got)
	}
}
