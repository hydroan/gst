// Package logfield declares log fields more than one stream of the framework
// writes, each as one constructor fixing the field's key and its type. A log
// store that maps fields as they arrive types a key by the first entry carrying
// it and rejects every later entry carrying the key with another type, so a key
// two streams render differently loses one stream's entries without a word from
// either; for a key declared here the constructor is the one place its type is
// decided, and selfcheck holds the framework's own packages to it. A key
// several streams write as a literal, such as ip, is held to one type by the
// same check instead.
package logfield

import (
	"time"

	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/util"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// The keys declared here alone; the keys of the fields a request is logged
// with are the ones its context stores the same values under, in requestctx
// and consts, and the trace id's is execctx's.
const (
	keyStatus    = "status"
	keyThreshold = "threshold"

	// KeyInstance is the log field naming the process that wrote the entry,
	// the identity every logger stamps; see internal/instance.
	KeyInstance = "instance"
	// KeyQuery is the log field carrying the raw query string of a request,
	// see Query.
	KeyQuery = "query"
	// KeyDuration reports how long an operation took as integer nanoseconds,
	// KeyDurationHuman the same value rendered for reading ("2.12ms"). Every
	// elapsed time the framework logs goes through Duration, which is the
	// only writer of this pair, so one query reaches the timing of any
	// operation regardless of which component produced it.
	KeyDuration      = "duration"
	KeyDurationHuman = "duration_human"
)

// TimeLayout is the timestamp layout of every log entry, and the only one:
// no logger may override it, or its file alone would become unorderable
// against the rest.
//
// RFC3339 with nanoseconds is what makes an entry's own timestamp usable.
// Whole seconds cannot order the entries of a single request, which pushes
// readers onto whatever ingest time their log pipeline stamps on, and that
// measures the pipeline rather than the work. Entries are rendered in UTC
// (see the zap encoder): the framework stores the UTC wall clock on every
// dialect, so a log line reads on the same clock as the rows it describes,
// entries from hosts in different zones order lexicographically, and no host
// zone leaks into the stream. Log stores recognize this layout as a date
// without being told, so the field is sortable and range-filterable on
// arrival.
const TimeLayout = time.RFC3339Nano

// Route is the registered route a request matched, /api/records/:record.
func Route(route string) zap.Field { return zap.String(requestctx.KeyRoute, route) }

// Path is the path of the one request, /api/records/42.
func Path(path string) zap.Field { return zap.String(requestctx.KeyPath, path) }

// Method is the HTTP method of the request; a gRPC call carries the method its
// rpc was registered with, STREAM for a Stream action's, and POST when the
// registration names none, as for the health and reflection services.
func Method(method string) zap.Field { return zap.String(requestctx.KeyMethod, method) }

// Username is the name of the authenticated caller, "" for an anonymous
// request.
func Username(username string) zap.Field { return zap.String(consts.CTX_USERNAME, username) }

// UserID is the id of the authenticated caller, "" for an anonymous request.
func UserID(id string) zap.Field { return zap.String(consts.CTX_USER_ID, id) }

// TenantID is the tenant the request acts in, the value its context carries
// under consts.CTX_TENANT_ID once the authenticated chain has run; "" for a
// request that established none.
func TenantID(id string) zap.Field { return zap.String(consts.CTX_TENANT_ID, id) }

// TraceID is the trace id of the request.
func TraceID(id string) zap.Field { return zap.String(execctx.KeyTraceID, id) }

// Query is the raw query string of the request, one string: its keys are the
// client's to choose, so an object of them would grow a store's mapping
// without bound.
func Query(raw string) zap.Field { return zap.String(KeyQuery, raw) }

// Params renders the route parameters as one object, a key per parameter:
// {"params":{"record":"42"}}. The keys come from the registered routes and
// are therefore bounded, which is what makes an object, rather than the one
// string an unbounded map would have to be, safe for a store that maps every
// key.
func Params(params map[string]string) zap.Field {
	return zap.Object(requestctx.KeyParams, paramsObject(params))
}

type paramsObject map[string]string

func (o paramsObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	for k, v := range o {
		enc.AddString(k, v)
	}
	return nil
}

// Status is the HTTP status code a response carried, a number:
// {"status":404}. The gRPC listener's access log names the code of a call
// grpc_code instead, the two codes sharing no number space.
func Status(code int) zap.Field { return zap.Int(keyStatus, code) }

// Threshold is the duration a slow operation was measured against, rendered
// as the integer nanoseconds Duration renders a duration in:
// {"threshold":200000000}.
func Threshold(d time.Duration) zap.Field { return zap.Duration(keyThreshold, d) }

// Duration is the only supported way to log how long an operation took. It
// emits one field pair, integer nanoseconds plus the same value rendered for
// reading:
//
//	{"msg": "executed query", "duration": 2120000, "duration_human": "2.12ms"}
//
// Both halves are needed and neither pays for the other. Log stores index the
// nanosecond field as an integer, so "slower than 500ms" is a range filter and
// averages and percentiles are aggregations; because time.Duration is integral
// there is never a mix of integer and floating point values for the store to
// reconcile, and no unit is left implicit in the field name. The rendered
// string is what a person reads while tailing a log, where raw nanoseconds are
// unreadable.
//
// The keys are KeyDuration and KeyDurationHuman and are written here alone,
// so no call site can spell them differently and no query has to know which
// component produced the entry.
func Duration(d time.Duration) zap.Field {
	return zap.Inline(duration(d))
}

// duration renders an elapsed time as the field pair documented on Duration.
// It is inlined rather than nested so both fields sit at the top level of the
// entry, where the field name alone locates them.
type duration time.Duration

func (d duration) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt64(KeyDuration, int64(d))
	enc.AddString(KeyDurationHuman, util.FormatDurationSmart(time.Duration(d)))
	return nil
}
