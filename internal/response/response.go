package response

import (
	"encoding/json"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/serviceregistry"
)

// SuccessMsg is the msg of the success envelope, the one JSON writes.
const SuccessMsg = "success"

// The answer of the generic failure: an error that is not a service error,
// one no status and client-safe message were chosen for. Its text stays out
// of the envelope.
const (
	failureStatus = http.StatusBadRequest
	failureMsg    = "The request could not be processed."
)

// JSON writes data in the success envelope: status 200, SuccessMsg, data
// (null when none is given) and the trace id.
func JSON(c *gin.Context, data ...any) {
	var payload any
	if len(data) > 0 {
		payload = data[0]
	}
	envelope(c, http.StatusOK, SuccessMsg, payload)
}

// Error writes err in the failure envelope: a service error, anywhere in the
// wrap chain, answers with the status and client-safe message it was
// constructed with; any other error answers the generic failure, 400 with
// failureMsg. Internal error text — database drivers naming tables and
// columns, third-party client output — never reaches the envelope; callers
// log the full error themselves before answering it here.
func Error(c *gin.Context, err error) {
	var serviceErr *serviceregistry.Error
	if errors.As(err, &serviceErr) {
		envelope(c, serviceErr.Status(), serviceErr.Msg(), nil)
		return
	}
	envelope(c, failureStatus, failureMsg, nil)
}

// Abort refuses the request with status and msg, written in the failure
// envelope, and stops the handler chain.
//
// It exists so that code outside the controller path — middleware, and the
// middleware a module ships to the projects that copy it — has one way to
// refuse: what the envelope looks like on the wire is the framework's to
// decide and to change, and a caller here states only the refusal it is
// making.
func Abort(c *gin.Context, status int, msg string) {
	c.Abort()
	envelope(c, status, msg, nil)
}

// envelope writes the API envelope — msg, data and the trace id — with
// status. It is encoded with encoding/json whatever JSON codec gin was built
// with: the codecs gin's jsoniter, go_json and sonic build tags select encode
// differently (the first two ignore omitzero), and the framework's wire
// contract is the encoding/json one.
func envelope(c *gin.Context, status int, msg string, data any) {
	c.Render(status, jsonRender{data: gin.H{
		"msg":           msg,
		"data":          data,
		consts.TRACE_ID: c.GetString(consts.TRACE_ID),
	}})
}

// jsonContentType is the Content-Type of a JSON response, the value gin's own
// JSON render sets.
var jsonContentType = []string{"application/json; charset=utf-8"}

// jsonRender renders data as JSON through encoding/json. It mirrors gin's JSON
// render in everything but the codec: the Content-Type is set only when none
// is set yet, and a marshaling failure is returned before anything is written,
// for gin.Context.Render to record and abort on.
type jsonRender struct {
	data any
}

// Render writes the JSON Content-Type and the encoded data.
func (r jsonRender) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	body, err := json.Marshal(r.data)
	if err != nil {
		return err
	}
	_, err = w.Write(body)
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
