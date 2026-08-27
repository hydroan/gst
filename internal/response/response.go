package response

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
)

// Success / failure sentinel codes.
const (
	CodeSuccess Code = 0
	CodeFailure Code = -1
)

// General API error codes.
const (
	CodeInvalidParam Code = 1000 + iota
	CodeNotFound
	CodeAlreadyExist
	CodeStaleObject
)

type codeValue struct {
	Status int
	Msg    string
}

// defaultCodeValueMap is the built-in mapping from Code to HTTP status and default message.
var defaultCodeValueMap = map[Code]codeValue{
	CodeSuccess: {http.StatusOK, "success"},
	CodeFailure: {http.StatusBadRequest, "failure"},

	// General codes
	CodeInvalidParam: {http.StatusBadRequest, "Invalid parameters provided in the request."},
	CodeNotFound:     {http.StatusNotFound, "Requested resource not found."},
	CodeAlreadyExist: {http.StatusConflict, "Resource already exists."},
	CodeStaleObject:  {http.StatusConflict, "Resource was modified by another operation. Reload and retry."},
}

// customCodeValueMap holds app-defined overrides from Code to HTTP status and message.
var customCodeValueMap = make(map[Code]codeValue)

// Code is a stable numeric API error code.
type Code int32

// CodeInstance is a Code with optional per-response HTTP status and message overrides.
// Nil pointer fields mean "use the value from Code (including customCodeValueMap / defaultCodeValueMap)".
type CodeInstance struct {
	code   Code
	status *int
	msg    *string
}

var (
	_ types.Coder = Code(0)
	_ types.Coder = CodeInstance{}
)

// lookup returns the configured status and message for r from custom then default maps.
func (r Code) lookup() (codeValue, bool) {
	if val, ok := customCodeValueMap[r]; ok {
		return val, true
	}
	if val, ok := defaultCodeValueMap[r]; ok {
		return val, true
	}
	return codeValue{}, false
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
	return defaultCodeValueMap[CodeFailure].Msg
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

func NewCode(code Code, status int, msg string) Code {
	customCodeValueMap[code] = codeValue{
		Status: status,
		Msg:    msg,
	}
	return code
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

func JSON(c *gin.Context, coder types.Coder, data ...any) {
	// Record the envelope code so post-response middleware (e.g. the HTTP
	// body logger) can classify the outcome even when the HTTP status is 2xx.
	c.Set(consts.CTX_RESPONSE_CODE, coder.Code())
	if len(data) > 0 {
		c.JSON(coder.Status(), gin.H{
			"code":          coder.Code(),
			"msg":           coder.Msg(),
			"data":          data[0],
			consts.TRACE_ID: c.GetString(consts.TRACE_ID),
		})
	} else {
		c.JSON(coder.Status(), gin.H{
			"code":          coder.Code(),
			"msg":           coder.Msg(),
			"data":          nil,
			consts.TRACE_ID: c.GetString(consts.TRACE_ID),
		})
	}
}

func Bytes(c *gin.Context, coder types.Coder, data ...[]byte) {
	c.Set(consts.CTX_RESPONSE_CODE, coder.Code())
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("X-cached", "true")
	var dataStr string
	if len(data) > 0 {
		dataStr = fmt.Sprintf(`{"code":%d,"msg":"%s","data":%s,"trace_id":"%s"}`, coder.Code(), coder.Msg(), util.BytesToString(data[0]), c.GetString(consts.TRACE_ID))
	} else {
		dataStr = fmt.Sprintf(`{"code":%d,"msg":"%s","data":"","trace_id":"%s"}`, coder.Code(), coder.Msg(), c.GetString(consts.TRACE_ID))
	}
	c.Writer.WriteHeader(coder.Status())
	_, _ = c.Writer.Write(util.StringToBytes(dataStr))
}

func BytesList(c *gin.Context, coder types.Coder, total int, data ...[]byte) {
	c.Set(consts.CTX_RESPONSE_CODE, coder.Code())
	c.Header("Content-Type", "application/json; charset=utf-8")
	var dataStr string
	if len(data) > 0 {
		dataStr = fmt.Sprintf(`{"code":%d,"msg":"%s","data":{"total":%d,"items":%s},"trace_id":"%s"}`, coder.Code(), coder.Msg(), total, util.BytesToString(data[0]), c.GetString(consts.TRACE_ID))
	} else {
		dataStr = fmt.Sprintf(`{"code":%d,"msg":"%s","data":{"total":0,"items":[]},"trace_id":"%s"}`, coder.Code(), coder.Msg(), c.GetString(consts.TRACE_ID))
	}
	c.Writer.WriteHeader(coder.Status())
	_, _ = c.Writer.Write(util.StringToBytes(dataStr))
}

func Text(c *gin.Context, coder types.Coder, data ...any) {
	c.Set(consts.CTX_RESPONSE_CODE, coder.Code())
	if len(data) > 0 {
		c.String(coder.Status(), stringAny(data))
	} else {
		c.String(coder.Status(), "")
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

func File(c *gin.Context, filename string) {
	c.Set(consts.CTX_RESPONSE_CODE, CodeSuccess.Code())
	c.File(filename)
}

func stringAny(v any) string {
	if v == nil {
		return ""
	}
	val, ok := v.(fmt.Stringer)
	if ok {
		return val.String()
	}

	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case []string:
		return strings.Join(val, ",")
	case [][]byte:
		return string(bytes.Join(val, []byte(",")))
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}
