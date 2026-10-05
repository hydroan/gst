package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/hydroan/gst/internal/consts"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// newLogLevel parses the configured level; defaults to Info.
func newLogLevel(cfg logConfig) zapcore.Level {
	if len(cfg.level) == 0 {
		return zapcore.InfoLevel
	}
	level := new(zapcore.Level)
	if err := level.UnmarshalText([]byte(cfg.level)); err != nil {
		return zapcore.InfoLevel
	}
	return *level
}

// newLogEncoder builds the JSON or console encoder cfg names, with the
// fields opts leave out.
func newLogEncoder(cfg logConfig, opts ...Option) zapcore.Encoder {
	encConfig := zap.NewProductionEncoderConfig()
	// The entry timestamp is rendered in UTC using consts.LayoutTimeEncoder.
	// The conversion happens here, the single point every logger's encoder is
	// built at, so no host zone can leak into a log entry; see the layout
	// constant for why the stream is UTC.
	encConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.UTC().Format(consts.LayoutTimeEncoder))
	}
	encConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	// Nanoseconds, matching util.LogDuration. The zap production default encodes
	// durations as floating point seconds, which rounds sub-millisecond work to
	// zero and leaves a log store guessing between integer and float. This
	// backstops any time.Duration that reaches a logger without going through
	// util.LogDuration, so no entry can carry a differently scaled duration.
	encConfig.EncodeDuration = zapcore.NanosDurationEncoder
	// Reflected objects and arrays collapse into a single JSON string field
	// instead of nesting; see stringifyReflectedEncoder for why.
	encConfig.NewReflectedEncoder = newStringifyReflectedEncoder
	if len(opts) > 0 {
		o := opts[0]
		if o.DisableMsg {
			encConfig.MessageKey = ""
		}
		if o.DisableLevel {
			encConfig.LevelKey = ""
		}
		if o.DisableCaller {
			encConfig.CallerKey = ""
		}
	}
	switch strings.ToLower(cfg.format) {
	case "json":
		return zapcore.NewJSONEncoder(encConfig)
	case "text", "console":
		return zapcore.NewConsoleEncoder(encConfig)
	default:
		return zapcore.NewJSONEncoder(encConfig)
	}
}

// stringifyReflectedEncoder renders a reflected log value whose JSON is an
// object or an array as a single JSON string instead of nesting it. zap falls
// back to reflection for values no typed Field constructor understands:
// structs, maps, slices or arrays of either, and named types such as
// `type Mode string` whose underlying type it would understand. The default
// reflected encoder inlines their JSON shape into the entry. A log store that
// indexes each key then grows its field mapping with every distinct shape
// logged anywhere in the codebase and eventually hits its per-index field
// cap, after which it drops entries. Collapsing the value into one string
// field keeps the mapping bounded no matter what gets logged, and the string
// still carries the value's JSON, so the content stays machine-readable. A
// value whose JSON is a scalar — a string, number, boolean or null — adds a
// single key whatever its Go type, so it is written as it is and keeps the
// native JSON type a typed field would give it. Typed fields and
// zapcore.ObjectMarshaler implementations are unaffected; the marshaler
// escape hatch is reserved for framework-internal objects whose key sets are
// fixed, never for open-ended shapes such as data models.
type stringifyReflectedEncoder struct{ w io.Writer }

// newStringifyReflectedEncoder is the zapcore.EncoderConfig.NewReflectedEncoder
// hook wired by newLogEncoder, the single point every logger's encoder is
// built at, so all loggers share the bounded-field behavior.
func newStringifyReflectedEncoder(w io.Writer) zapcore.ReflectedEncoder {
	return stringifyReflectedEncoder{w: w}
}

// reflectedValueBuffers pools the buffer Encode first writes a value's JSON
// into. Reflected values reach the encoder on every request that logs one, so
// the buffer would otherwise be allocated per entry.
var reflectedValueBuffers = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// maxPooledReflectedValueBytes caps the buffer a finished Encode hands back.
// sync.Pool assumes its entries cost about the same, so a single oversized
// value would otherwise pin its buffer for the life of the process. The limit
// matches the one the standard library's fmt package applies to its own pooled
// output buffer, and is far above any value worth reading back in a log entry.
// See https://go.dev/issue/23199.
const maxPooledReflectedValueBytes = 64 * 1024

// releaseReflectedValueBuffer returns buf to the pool unless it outgrew the
// pooled size, in which case it is dropped for the garbage collector.
func releaseReflectedValueBuffer(buf *bytes.Buffer) {
	if buf.Cap() > maxPooledReflectedValueBytes {
		return
	}

	buf.Reset()
	reflectedValueBuffers.Put(buf)
}

// Encode implements zapcore.ReflectedEncoder. It writes the value's JSON into
// a pooled buffer. A scalar is then written as it is; an object or an array is
// written as one JSON string, so the entry gains a single string field. A
// value json cannot handle (cycles, channels, functions) falls back to Go
// syntax, as a string too, rather than failing the whole entry.
//
// Every pass leaves <, > and & as written. HTML escaping exists to keep JSON
// safe inside an HTML document, and a log entry is never rendered as one: it
// is consumed by log stores and by people reading them. The only effect here
// would be turning those three characters into six-character escapes, and they
// appear densely in exactly the payloads worth reading back — third-party
// error pages, URLs and query strings. Escaping has to be off on both passes
// of an object or an array: the second one re-escapes the characters the
// first one left alone, so disabling it on the value pass by itself changes
// nothing.
func (e stringifyReflectedEncoder) Encode(value any) error {
	buf, _ := reflectedValueBuffers.Get().(*bytes.Buffer)
	defer releaseReflectedValueBuffer(buf)

	if err := newVerbatimJSONEncoder(buf).Encode(value); err != nil {
		buf.Reset()
		fmt.Fprintf(buf, "%#v", value)
	} else if first := buf.Bytes()[0]; first != '{' && first != '[' {
		// zapcore trims the newline Encode terminates the JSON with.
		_, err = e.w.Write(buf.Bytes())
		return err
	}

	// Encode terminates its output with a newline, which must not travel into
	// the quoted string; zapcore trims the one this second pass appends.
	return newVerbatimJSONEncoder(e.w).Encode(string(bytes.TrimRight(buf.Bytes(), "\n")))
}

// newVerbatimJSONEncoder returns a json encoder that emits <, > and & as
// written instead of escaping them.
func newVerbatimJSONEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	return enc
}
