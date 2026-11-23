package response

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"net/http"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/i18n"
	"github.com/hydroan/gst/internal/types"
)

// SuccessMsg is the msg of the success envelope, the one JSON writes.
const SuccessMsg = "success"

// The answer of the server's own failure: an error that is not a service
// error, one no status and client-safe message were chosen for, which is the
// server failing to serve the request and not the client's doing. Its text
// stays out of the envelope.
const (
	failureStatus = http.StatusInternalServerError
	failureMsg    = types.FailureMsg
)

// JSON writes data in the success envelope: status 200, SuccessMsg, data
// (null when none is given) and the trace id.
func JSON(c *gin.Context, data ...any) {
	var payload any
	if len(data) > 0 {
		payload = data[0]
	}
	envelope(c, http.StatusOK, SuccessMsg, payload, nil)
}

// Error writes err in the failure envelope: a service error, anywhere in the
// wrap chain, answers with the status and client-safe message it was
// constructed with and, when it names the fields the request was refused on
// (see types.FieldViolations), with those fields in the error member of the
// envelope, the list a gRPC status carries as its google.rpc.BadRequest
// detail:
//
//	{"data":null,"msg":"items[1].name is a required field","trace_id":"...","error":{"field_violations":[{"field":"items[1].name","description":"items[1].name is a required field"}]}}
//
// The data of a failure is null, a failure carrying no details has no error
// member, and any other error is the server's own failure and answers 500
// with failureMsg. The message is answered in the language the process
// answers in (see i18n.Translate), the one a gRPC status carries it in.
// Internal error text — database drivers naming tables and columns,
// third-party client output — never reaches the envelope; callers log the
// full error themselves before answering it here.
func Error(c *gin.Context, err error) {
	var serviceErr *types.Error
	if errors.As(err, &serviceErr) {
		var details *errorDetails
		if violations := types.FieldViolations(serviceErr); len(violations) > 0 {
			details = &errorDetails{FieldViolations: violations}
		}
		envelope(c, serviceErr.Status(), i18n.Translate(serviceErr.Msg()), nil, details)
		return
	}
	envelope(c, failureStatus, i18n.Translate(failureMsg), nil, nil)
}

// Abort refuses the request with err, written in the failure envelope the
// way Error writes it, and stops the handler chain: what code outside the
// controller path refuses a request through, and nowhere else. A refusal with a
// status and message of its own is a service error, types.NewError(status,
// msg), so the one rule NewError holds for statuses and messages holds for
// every refusal: what the envelope looks like on the wire is the framework's to
// decide and to change, and a caller here states only the refusal it is making.
// The public response.Abort forwards to it.
func Abort(c *gin.Context, err error) {
	c.Abort()
	Error(c, err)
}

// envelopeBufferCap is the largest buffer envelope keeps for the response
// after: one a larger body grew is left to the garbage collector, so a
// single large response does not hold its size for as long as the process
// runs. It is the bound fmt keeps on the buffers it uses again.
const envelopeBufferCap = 64 << 10

// envelopeBuffers holds the buffers envelopes are encoded into, so a
// response does not allocate its body anew: json.Marshal returns a copy of
// the body, of the body's size, on every call.
var envelopeBuffers = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// envelope writes the API envelope — msg, data, the trace id and, for a
// failure carrying them, its details — with status. It is encoded with
// encoding/json whatever JSON codec gin was built
// with: the codecs gin's jsoniter, go_json and sonic build tags select encode
// differently (the first two ignore omitzero), and the framework's wire
// contract is the encoding/json one, written here as json.Marshal writes it,
// encoding/json/v2 under the options json.DefaultOptionsV1 names. It is
// encoded before anything is written: a data no JSON holds, a NaN or a
// channel a service put there, is the server's own failure, answered 500
// with failureMsg, translated the way Error translates it, and recorded on
// the context for the access log, the handler chain stopped, where an
// encoding failing under gin's render would leave the status already
// written, a 200 with an empty body that reads as a success with nothing in
// it.
func envelope(c *gin.Context, status int, msg string, data any, details *errorDetails) {
	traceID := c.GetString(execctx.KeyTraceID)
	// Nearly every answer carries no details, and an envelope type holding
	// the error member costs each of them its encoding, so only a failure
	// carrying details is encoded as the envelope with the member.
	var answer any = apiEnvelope{Data: data, Msg: msg, TraceID: traceID}
	if details != nil {
		answer = detailedEnvelope{Data: data, Msg: msg, TraceID: traceID, Error: details}
	}
	body := envelopeBuffers.Get().(*bytes.Buffer) //nolint:errcheck // the pool holds nothing else
	if err := jsonv2.MarshalWrite(body, answer, json.DefaultOptionsV1()); err != nil {
		_ = c.Error(err)
		c.Abort()
		status = failureStatus
		body.Reset()
		_ = jsonv2.MarshalWrite(body, apiEnvelope{Msg: i18n.Translate(failureMsg), TraceID: traceID}, json.DefaultOptionsV1()) // strings and a null: nothing in it fails to encode
	}
	c.Render(status, jsonRender{body: body.Bytes()})

	// The render has written the body out, and a writer keeps nothing of
	// what it is handed, so the buffer is free for the response after.
	if body.Cap() <= envelopeBufferCap {
		body.Reset()
		envelopeBuffers.Put(body)
	}
}

// apiEnvelope is the API envelope as the wire carries it: data, msg and the
// trace id, in this key order. A response that could not be encoded is
// answered with the failure message, a null data and the trace id, nothing
// in it that fails to encode, so that encoding goes unchecked.
type apiEnvelope struct {
	Data    any    `json:"data"`
	Msg     string `json:"msg"`
	TraceID string `json:"trace_id"`
}

// detailedEnvelope is the envelope of a failure carrying details: the
// members of every envelope and, after them, error.
type detailedEnvelope struct {
	apiEnvelope

	Error *errorDetails `json:"error"`
}

// errorDetails is the error member of a failure envelope: what the failure
// carries beyond its msg, the fields the request was refused on.
type errorDetails struct {
	FieldViolations []types.FieldViolation `json:"field_violations,omitempty"`
}

// jsonContentType is the Content-Type of a JSON response, the value gin's own
// JSON render sets.
var jsonContentType = []string{"application/json; charset=utf-8"}

// jsonRender renders an encoded JSON body. It mirrors gin's JSON render in
// everything but the codec, the body being encoded by envelope already: the
// Content-Type is set only when none is set yet.
type jsonRender struct {
	body []byte
}

// Render writes the JSON Content-Type and the body.
func (r jsonRender) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	_, err := w.Write(r.body)
	return err
}

// WriteContentType sets the JSON Content-Type unless one is already set.
func (jsonRender) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	if len(header["Content-Type"]) == 0 {
		header["Content-Type"] = jsonContentType
	}
}

// Attachment writes data as a downloadable file, setting the download file name
// and content type explicitly. It is used for exports where the format decides
// the file extension and MIME type.
func Attachment(c *gin.Context, data []byte, filename, contentType string) {
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, contentType, data)
}
