package response

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/sse"
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
	CodeBadRequest
	CodeInvalidToken
	CodeNeedLogin
	CodeUnauthorized
	CodeNetworkTimeout
	CodeContextTimeout
	CodeTooManyRequests
	CodeNotFound
	CodeForbidden
	CodeAlreadyExist
)

// Domain / business error codes.
const (
	CodeInvalidLogin Code = 2000 + iota
	CodeInvalidSignup
	CodeOldPasswordNotMatch
	CodeNewPasswordNotMatch

	CodeNotFoundQueryID
	CodeNotFoundRouteParam
	CodeNotFoundUser
	CodeNotFoundUserID

	CodeAlreadyExistsUser
	CodeAlreadyExistsRole

	CodeTooLargeFile
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
	CodeInvalidParam:    {http.StatusBadRequest, "Invalid parameters provided in the request."},
	CodeBadRequest:      {http.StatusBadRequest, "Malformed or illegal request."},
	CodeInvalidToken:    {http.StatusUnauthorized, "Invalid or expired authentication token."},
	CodeNeedLogin:       {http.StatusUnauthorized, "Authentication required to access the requested resource."},
	CodeUnauthorized:    {http.StatusUnauthorized, "Unauthorized access to the requested resource."},
	CodeNetworkTimeout:  {http.StatusGatewayTimeout, "Network operation timed out."},
	CodeContextTimeout:  {http.StatusGatewayTimeout, "Request context timed out."},
	CodeTooManyRequests: {http.StatusTooManyRequests, "too many requests, please try again later."},
	CodeNotFound:        {http.StatusNotFound, "Requested resource not found."},
	CodeForbidden:       {http.StatusForbidden, "Forbidden: Inadequate privileges for the requested operation."},
	CodeAlreadyExist:    {http.StatusConflict, "Resource already exists."},

	// Business codes
	CodeInvalidLogin:        {http.StatusBadRequest, "invalid username or password"},
	CodeInvalidSignup:       {http.StatusBadRequest, "invalid username or password"},
	CodeOldPasswordNotMatch: {http.StatusBadRequest, "old password not match"},
	CodeNewPasswordNotMatch: {http.StatusBadRequest, "new password not match"},
	CodeNotFoundQueryID:     {http.StatusBadRequest, "not found query parameter 'id'"},
	CodeNotFoundRouteParam:  {http.StatusBadRequest, "not found router param"},
	CodeNotFoundUser:        {http.StatusBadRequest, "not found user"},
	CodeNotFoundUserID:      {http.StatusBadRequest, "not found user id"},
	CodeAlreadyExistsUser:   {http.StatusConflict, "user already exists"},
	CodeAlreadyExistsRole:   {http.StatusConflict, "role already exists"},
	CodeTooLargeFile:        {http.StatusBadRequest, "too large file"},
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

func (r Code) WithStatus(status int) CodeInstance {
	return CodeInstance{code: r, status: &status, msg: nil}
}

func (r Code) WithErr(err error) CodeInstance {
	msg := err.Error()
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
	msg := err.Error()
	return CodeInstance{code: ci.code, status: ci.status, msg: &msg}
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
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, contentType, data)
}

func File(c *gin.Context, filename string) {
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

// SSE sends a Server-Sent Events (SSE) response.
// This function sets the appropriate headers for SSE and writes the event to the response.
//
// Note: This function sends a single event, not a stream. If you need to send a [DONE] marker
// after this event (e.g., for AI chat completions), you should use sse.EncodeDone() or
// call SendSSEDone() if available in your context.
//
// Parameters:
//   - c: Gin context
//   - event: SSE event to send
//
// Example:
//
//	SSE(c, sse.Event{
//	    Event: "message",
//	    Data:  "Hello, World!",
//	})
func SSE(c *gin.Context, event sse.Event) error {
	return sse.SendSSE(c.Writer, event)
}

// StreamSSE starts a Server-Sent Events stream.
// The provided function will be called repeatedly until it returns false.
// The stream will automatically stop if:
//   - The function returns false
//   - The request context is canceled (timeout, client disconnect, etc.)
//   - An error occurs while writing to the client
//
// Note: This function does NOT automatically send a [DONE] marker when the stream ends.
// If your protocol requires a [DONE] marker (e.g., AI chat completions), you must
// manually call sse.EncodeDone(c.Writer) after StreamSSE() returns.
//
// Parameters:
//   - c: Gin context
//   - fn: Function that sends events. Returns false to stop streaming.
//     The function receives the writer and should check context cancellation if needed.
//
// Example:
//
//	StreamSSE(c, func(w io.Writer) bool {
//	    sse.Encode(w, sse.Event{
//	        Event: "message",
//	        Data:  "Hello",
//	    })
//	    return true // Continue streaming
//	})
//	// Send [DONE] marker if required by your protocol
//	sse.EncodeDone(c.Writer)
func StreamSSE(c *gin.Context, fn func(io.Writer) bool) {
	sse.StreamSSE(c.Request.Context(), c.Writer, c.Stream, fn)
}

// StreamSSEWithInterval starts a Server-Sent Events stream with a fixed interval between events.
// The provided function will be called repeatedly at the specified interval until it returns false.
// The stream will automatically stop if:
//   - The function returns false
//   - The request context is canceled (timeout, client disconnect, etc.)
//   - An error occurs while writing to the client
//
// Note: This function does NOT automatically send a [DONE] marker when the stream ends.
// If your protocol requires a [DONE] marker (e.g., AI chat completions), you must
// manually call sse.EncodeDone(c.Writer) after StreamSSEWithInterval() returns.
//
// Parameters:
//   - c: Gin context
//   - interval: Time interval between events
//   - fn: Function that sends events. Returns false to stop streaming.
//     The function receives the writer and should check context cancellation if needed.
//
// Example:
//
//	StreamSSEWithInterval(c, 1*time.Second, func(w io.Writer) bool {
//	    sse.Encode(w, sse.Event{
//	        Event: "message",
//	        Data:  time.Now().String(),
//	    })
//	    return true // Continue streaming
//	})
//	// Send [DONE] marker if required by your protocol
//	sse.EncodeDone(c.Writer)
func StreamSSEWithInterval(c *gin.Context, interval time.Duration, fn func(io.Writer) bool) {
	sse.StreamSSEWithInterval(c.Request.Context(), c.Writer, c.Stream, interval, fn)
}
