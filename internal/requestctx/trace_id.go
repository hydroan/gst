package requestctx

import (
	"strings"

	"github.com/hydroan/gst/internal/util"
	"go.opentelemetry.io/otel/trace"
)

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
	return util.TraceID()
}
