package logger

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestNewLogEncoderTimestampIsUTCAndOrdersWithinASecond(t *testing.T) {
	encoder := newLogEncoder(readConf(""))
	at := time.Date(2026, 7, 29, 14, 3, 8, 243834831, time.FixedZone("", 8*60*60))

	encode := func(at time.Time) string {
		buf, err := encoder.EncodeEntry(zapcore.Entry{Time: at}, nil)
		require.NoError(t, err)
		var entry struct {
			TS string `json:"ts"`
		}
		require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
		return entry.TS
	}

	first := encode(at)
	require.Equal(t, "2026-07-29T06:03:08.243834831Z", first,
		"the host zone must not leak into the timestamp")

	// The entries of one request land in the same second, so a whole-second
	// timestamp would collapse them into a single value and lose their order.
	next := encode(at.Add(time.Microsecond))
	require.Less(t, first, next)

	// The timestamp is rendered in UTC no matter what zone the host runs in,
	// so an entry reads on the same clock as the stored rows it describes, and
	// entries from any two hosts order lexicographically. The layout is one a
	// log store reads as a date unassisted.
	parsed, err := time.Parse(time.RFC3339Nano, first)
	require.NoError(t, err)
	require.True(t, parsed.Equal(at))
	_, offset := parsed.Zone()
	require.Equal(t, 0, offset)
}

func TestNewLogEncoderReflectedObjectsAndArraysCollapseToOneStringField(t *testing.T) {
	encoder := newLogEncoder(readConf(""))

	encode := func(t *testing.T, fields ...zapcore.Field) map[string]any {
		t.Helper()
		buf, err := encoder.EncodeEntry(zapcore.Entry{Time: time.Now()}, fields)
		require.NoError(t, err)
		var entry map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &entry),
			"every entry must stay one valid JSON document")
		return entry
	}

	type sampleRecord struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	// The struct, map, and slice shapes below all reach the encoder through
	// reflection, which zap.Reflect asks for and a sugared logger's key-value
	// pair falls back to for a value zap has no typed field for. Each must
	// land as a single string field so the log store's field mapping stays
	// bounded; the string still parses as the value's JSON.
	t.Run("struct", func(t *testing.T) {
		entry := encode(t, zap.Reflect("record", &sampleRecord{Name: "sample", Count: 3}))
		collapsed, ok := entry["record"].(string)
		require.True(t, ok, "reflected struct must encode as one string field, got %T", entry["record"])
		var record sampleRecord
		require.NoError(t, json.Unmarshal([]byte(collapsed), &record))
		require.Equal(t, sampleRecord{Name: "sample", Count: 3}, record)
	})

	t.Run("map", func(t *testing.T) {
		entry := encode(t, zap.Reflect("attributes", map[string]int{"total": 7}))
		collapsed, ok := entry["attributes"].(string)
		require.True(t, ok, "reflected map must encode as one string field, got %T", entry["attributes"])
		require.JSONEq(t, `{"total":7}`, collapsed)
	})

	t.Run("slice of structs", func(t *testing.T) {
		entry := encode(t, zap.Reflect("records", []sampleRecord{{Name: "first", Count: 1}}))
		collapsed, ok := entry["records"].(string)
		require.True(t, ok, "reflected slice must encode as one string field, got %T", entry["records"])
		require.JSONEq(t, `[{"name":"first","count":1}]`, collapsed)
	})

	t.Run("unmarshalable value falls back to Go syntax", func(t *testing.T) {
		entry := encode(t, zap.Reflect("stream", map[string]chan int{"items": make(chan int)}))
		collapsed, ok := entry["stream"].(string)
		require.True(t, ok, "fallback must still encode as one string field, got %T", entry["stream"])
		require.Contains(t, collapsed, "chan int")
	})

	// Angle brackets and ampersands survive both encoding passes. They are
	// dense in third-party error pages, URLs and query strings, and a log entry
	// is never rendered as HTML, so escaping them would only cost readability.
	t.Run("html characters stay verbatim", func(t *testing.T) {
		const raw = `<h2>Bad Gateway</h2> a&b`
		entry := encode(t, zap.Reflect("payload", map[string]string{"body": raw}))
		collapsed, ok := entry["payload"].(string)
		require.True(t, ok, "reflected map must encode as one string field, got %T", entry["payload"])
		require.Contains(t, collapsed, raw, "html characters must not be escaped")

		var payload map[string]string
		require.NoError(t, json.Unmarshal([]byte(collapsed), &payload),
			"the collapsed string must still parse as the value's JSON")
		require.Equal(t, raw, payload["body"])
	})

	// A value larger than the pooled buffer size takes the path that drops the
	// buffer instead of recycling it; the entry it produces must be unaffected.
	t.Run("value larger than the pooled buffer size", func(t *testing.T) {
		oversized := strings.Repeat("x", maxPooledReflectedValueBytes+1)
		entry := encode(t, zap.Reflect("payload", map[string]string{"body": oversized}))
		collapsed, ok := entry["payload"].(string)
		require.True(t, ok, "reflected map must encode as one string field, got %T", entry["payload"])

		var payload map[string]string
		require.NoError(t, json.Unmarshal([]byte(collapsed), &payload))
		require.Equal(t, oversized, payload["body"])
	})

	// Typed fields never touch the reflected encoder: scalar key-value pairs
	// keep their native JSON types and stay individually indexable.
	t.Run("typed fields keep their native JSON types", func(t *testing.T) {
		entry := encode(t, zap.String("kind", "sample"), zap.Int("count", 3))
		require.Equal(t, "sample", entry["kind"])
		require.IsType(t, float64(0), entry["count"], "count must stay a native JSON number")
		require.InDelta(t, 3, entry["count"], 0)
	})

	// Named scalar types — enums declared as `type Mode string` and the like —
	// reach the encoder through the same reflection, a sugared logger's
	// key-value pair recognizing only the unnamed types. A scalar adds a single key whatever
	// its type, so each keeps the native JSON type its typed field would give
	// it rather than landing as a quoted copy of its JSON.
	t.Run("named scalar types keep their native JSON types", func(t *testing.T) {
		type sampleKind string
		type sampleLevel int
		type sampleFlag bool
		entry := encode(t,
			zap.Reflect("kind", sampleKind("sample")),
			zap.Reflect("level", sampleLevel(3)),
			zap.Reflect("enabled", sampleFlag(true)),
			zap.Reflect("status", sampleStatus{name: "active"}),
			zap.Reflect("record", (*sampleRecord)(nil)),
		)
		require.Equal(t, "sample", entry["kind"])
		require.IsType(t, float64(0), entry["level"], "level must stay a native JSON number")
		require.InDelta(t, 3, entry["level"], 0)
		require.Equal(t, true, entry["enabled"])
		// A value marshaling itself to a JSON scalar is a scalar too, and so
		// is the null a nil pointer encodes to.
		require.Equal(t, "active", entry["status"])
		require.Contains(t, entry, "record")
		require.Nil(t, entry["record"])
	})
}

// BenchmarkNewLogEncoderReflectedValue measures encoding an entry whose one
// field reaches the reflected encoder: a named scalar, written as it is, and a
// struct, collapsed into one string field.
func BenchmarkNewLogEncoderReflectedValue(b *testing.B) {
	type sampleKind string
	type sampleRecord struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	encoder := newLogEncoder(readConf(""))
	entry := zapcore.Entry{Time: time.Now(), Message: "sample"}

	for _, bc := range []struct {
		name  string
		field zapcore.Field
	}{
		{name: "named scalar", field: zap.Reflect("kind", sampleKind("sample"))},
		{name: "struct", field: zap.Reflect("record", sampleRecord{Name: "sample", Count: 3})},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				buf, err := encoder.EncodeEntry(entry, []zapcore.Field{bc.field})
				if err != nil {
					b.Fatal(err)
				}
				buf.Free()
			}
		})
	}
}

// sampleStatus marshals itself to a JSON string, the way an enum with a JSON
// form of its own does.
type sampleStatus struct{ name string }

func (s sampleStatus) MarshalJSON() ([]byte, error) { return json.Marshal(s.name) }
