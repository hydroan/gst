// Package logfield declares the log fields more than one stream of the
// framework writes, each as one constructor fixing the field's key and its
// type. A log store that maps fields as they arrive types a key by the first
// entry carrying it and rejects every later entry carrying the key with
// another type, so a key two streams render differently loses one stream's
// entries without a word from either; the constructor is the one place the
// type of a shared key is decided, and every stream writes the key through
// it.
package logfield

import (
	"time"

	"github.com/hydroan/gst/internal/consts"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// The keys declared here alone; the request keys are the consts the request
// context stores the same values under.
const (
	statusKey    = "status"
	thresholdKey = "threshold"
)

// Route is the registered route a request matched, /api/records/:record.
func Route(route string) zap.Field { return zap.String(consts.CTX_ROUTE, route) }

// Path is the path of the one request, /api/records/42.
func Path(path string) zap.Field { return zap.String(consts.CTX_PATH, path) }

// Method is the HTTP method of the request; a gRPC call carries POST, the
// method every call arrives by.
func Method(method string) zap.Field { return zap.String(consts.CTX_METHOD, method) }

// Username is the name of the authenticated caller, "" for an anonymous
// request.
func Username(username string) zap.Field { return zap.String(consts.CTX_USERNAME, username) }

// UserID is the id of the authenticated caller, "" for an anonymous request.
func UserID(id string) zap.Field { return zap.String(consts.CTX_USER_ID, id) }

// TraceID is the trace id of the request.
func TraceID(id string) zap.Field { return zap.String(consts.TRACE_ID, id) }

// Query is the raw query string of the request, one string: its keys are the
// client's to choose, so an object of them would grow a store's mapping
// without bound.
func Query(raw string) zap.Field { return zap.String(consts.QUERY, raw) }

// Params renders the route parameters as one object, a key per parameter:
// {"params":{"record":"42"}}. The keys come from the registered routes and
// are therefore bounded, which is what makes an object, rather than the one
// string an unbounded map would have to be, safe for a store that maps every
// key.
func Params(params map[string]string) zap.Field {
	return zap.Object(consts.PARAMS, paramsObject(params))
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
func Status(code int) zap.Field { return zap.Int(statusKey, code) }

// Threshold is the duration a slow operation was measured against, rendered
// as the integer nanoseconds util.LogDuration renders a duration in:
// {"threshold":200000000}.
func Threshold(d time.Duration) zap.Field { return zap.Duration(thresholdKey, d) }
