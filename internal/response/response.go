package response

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
)

// Success / failure sentinel codes.
const (
	CodeSuccess Code = 0
	CodeFailure Code = -1
)

// General API error codes.
const (
	CodeInvalidArgument Code = 1000 + iota
	CodeNotFound
	CodeAlreadyExists
	CodeStaleObject
)

type codeAnswer struct {
	Status int
	Msg    string
}

// codeAnswers is what every Code answers with, its HTTP status and default
// message.
var codeAnswers = map[Code]codeAnswer{
	CodeSuccess: {http.StatusOK, "success"},
	CodeFailure: {http.StatusBadRequest, "The request could not be processed."},

	// General codes
	CodeInvalidArgument: {http.StatusBadRequest, "The request contains invalid parameters."},
	CodeNotFound:        {http.StatusNotFound, "The requested resource was not found."},
	CodeAlreadyExists:   {http.StatusConflict, "The resource already exists."},
	CodeStaleObject:     {http.StatusConflict, "The resource was modified by another operation. Reload and retry."},
}

// Code is a stable numeric API error code.
type Code int32

// CodeInstance is a Code with optional per-response HTTP status and message
// overrides. Nil pointer fields mean "use what the Code answers with" (see
// codeAnswers).
type CodeInstance struct {
	code   Code
	status *int
	msg    *string
}

var (
	_ types.Coder = Code(0)
	_ types.Coder = CodeInstance{}
)

// lookup returns the status and message of r, and false for a code the
// built-in mapping does not know.
func (r Code) lookup() (codeAnswer, bool) {
	val, ok := codeAnswers[r]
	return val, ok
}

func (r Code) Code() int {
	return int(r)
}

func (r Code) Status() int {
	if v, ok := r.lookup(); ok {
		return v.Status
	}
	return http.StatusBadRequest
}

func (r Code) Msg() string {
	if v, ok := r.lookup(); ok {
		return v.Msg
	}
	return codeAnswers[CodeFailure].Msg
}

// String renders the code with its message so a Code value logged or
// formatted directly stays readable instead of printing as a bare integer.
func (r Code) String() string {
	return fmt.Sprintf("%s (code=%d)", r.Msg(), int32(r))
}

func (r Code) WithStatus(status int) CodeInstance {
	return CodeInstance{code: r, status: &status, msg: nil}
}

func (r Code) WithErr(err error) CodeInstance {
	msg := clientSafeErrorMessage(err)
	return CodeInstance{code: r, status: nil, msg: &msg}
}

func (r Code) WithMsg(msg string) CodeInstance {
	return CodeInstance{code: r, status: nil, msg: &msg}
}

func (ci CodeInstance) Code() int {
	return ci.code.Code()
}

func (ci CodeInstance) Status() int {
	if ci.status != nil {
		return *ci.status
	}
	return ci.code.Status()
}

func (ci CodeInstance) Msg() string {
	if ci.msg != nil {
		return *ci.msg
	}
	return ci.code.Msg()
}

func (ci CodeInstance) WithStatus(status int) CodeInstance {
	return CodeInstance{code: ci.code, status: &status, msg: ci.msg}
}

func (ci CodeInstance) WithErr(err error) CodeInstance {
	msg := clientSafeErrorMessage(err)
	return CodeInstance{code: ci.code, status: ci.status, msg: &msg}
}

// clientSafeErrorMessage returns the message WithErr renders in the response
// envelope. Service-layer errors anywhere in the wrap chain render their
// client-safe Msg, keeping the internal cause (reported by Error for logs)
// out of API responses; other errors keep rendering their full Error text.
func clientSafeErrorMessage(err error) string {
	var serviceErr *serviceregistry.Error
	if errors.As(err, &serviceErr) {
		return serviceErr.Msg()
	}
	return err.Error()
}

func (ci CodeInstance) WithMsg(msg string) CodeInstance {
	return CodeInstance{code: ci.code, status: ci.status, msg: &msg}
}

// Abort refuses the request with status and msg, written in the API envelope,
// and stops the handler chain.
//
// It exists so that code outside the controller path — middleware, and the
// middleware a module ships to the projects that copy it — has one way to
// refuse. Writing the envelope by hand instead works until the envelope grows:
// the response code recorded just below was added on this path and reached none
// of the hand-written ones, so every such refusal logged a code its own body
// contradicted.
func Abort(c *gin.Context, status int, msg string) {
	c.Abort()
	JSON(c, CodeFailure.WithStatus(status).WithMsg(msg))
}

// JSON writes coder and data in the API envelope. The envelope is encoded with
// encoding/json whatever JSON codec gin was built with: the codecs gin's
// jsoniter, go_json and sonic build tags select encode differently (the first
// two ignore omitzero), and the framework's wire contract is the encoding/json
// one.
func JSON(c *gin.Context, coder types.Coder, data ...any) {
	// Record the envelope code so post-response middleware (e.g. the HTTP
	// body logger) can classify the outcome even when the HTTP status is 2xx.
	c.Set(consts.CTX_RESPONSE_CODE, coder.Code())
	var payload any
	if len(data) > 0 {
		payload = data[0]
	}
	c.Render(coder.Status(), jsonRender{data: gin.H{
		"code":          coder.Code(),
		"msg":           coder.Msg(),
		"data":          payload,
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
	c.Set(consts.CTX_RESPONSE_CODE, CodeSuccess.Code())
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, contentType, data)
}
