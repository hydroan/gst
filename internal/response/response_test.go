package response_test

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	ginjson "github.com/gin-gonic/gin/codec/json"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/testutil/swap"
)

// TestJSONEncodesWithStandardLibrary pins the response envelope to
// encoding/json whatever JSON codec gin was built with. The codecs the
// jsoniter and go_json build tags select ignore omitzero, so an envelope
// encoded through gin's codec grows keys, such as a zero created_at, that the
// framework's models declare absent.
func TestJSONEncodesWithStandardLibrary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	swap.Value(t, &ginjson.API, ginjson.Core(swappedGinCodec{}))

	type sample struct {
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at,omitzero"`
	}
	tests := []struct {
		name string
		data []any
		want string
	}{
		{"with data", []any{&sample{Name: "sample"}}, `{"code":0,"data":{"name":"sample"},"msg":"success","trace_id":"trace-sample"}`},
		{"without data", nil, `{"code":0,"data":null,"msg":"success","trace_id":"trace-sample"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(consts.TRACE_ID, "trace-sample")

			response.JSON(c, response.CodeSuccess, tt.data...)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}
			if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q, want %q", got, "application/json; charset=utf-8")
			}
			if got := w.Body.String(); got != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestJSONKeepsContentTypeSetBeforehand pins that the envelope render, like
// gin's own JSON render, leaves a Content-Type already set on the response
// alone.
func TestJSONKeepsContentTypeSetBeforehand(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Header("Content-Type", "application/problem+json")

	response.JSON(c, response.CodeSuccess)

	if got := w.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/problem+json")
	}
}

// TestJSONWritesNoBodyForBodylessStatus pins the envelope on a status that
// cannot carry a body: the JSON Content-Type is still announced and nothing is
// written.
func TestJSONWritesNoBodyForBodylessStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	response.JSON(c, response.CodeSuccess.WithStatus(http.StatusNoContent))

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json; charset=utf-8")
	}
	if got := w.Body.String(); got != "" {
		t.Errorf("body = %q, want empty", got)
	}
}

// TestJSONRecordsMarshalFailureWithoutWritingBody pins the failure path of the
// envelope render: an envelope encoding/json cannot encode writes no partial
// body, and the error reaches the context's errors with the handler chain
// aborted, as with gin's own JSON render.
func TestJSONRecordsMarshalFailureWithoutWritingBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	response.JSON(c, response.CodeSuccess, math.NaN())

	if got := w.Body.String(); got != "" {
		t.Errorf("body = %q, want empty", got)
	}
	if len(c.Errors) != 1 {
		t.Errorf("context errors = %d, want 1", len(c.Errors))
	}
	if !c.IsAborted() {
		t.Error("handler chain was not aborted after the encoding failure")
	}
}

// swappedGinCodec stands in for the codec gin compiles in under the jsoniter,
// go_json or sonic build tags: whatever it encodes is recognizably not
// encoding/json's output, and the methods it leaves to the nil embedded Core
// panic when called.
type swappedGinCodec struct{ ginjson.Core }

func (swappedGinCodec) Marshal(any) ([]byte, error) { return []byte(`"swapped codec"`), nil }

func TestAttachment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	response.Attachment(c, []byte("hello"), "exported.csv", "text/csv; charset=utf-8")

	if got := w.Header().Get("Content-Disposition"); got != "attachment; filename=exported.csv" {
		t.Errorf("Content-Disposition = %q, want %q", got, "attachment; filename=exported.csv")
	}
	if got := w.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", got, "text/csv; charset=utf-8")
	}
	if got := w.Body.String(); got != "hello" {
		t.Errorf("body = %q, want %q", got, "hello")
	}
}

func TestWithErrKeepsServiceErrorCauseOutOfMessage(t *testing.T) {
	cause := errors.New("database password leaked")
	serviceErr := serviceregistry.NewErrorWithCause(http.StatusInternalServerError, "failed to load user", cause)

	// Both WithErr variants must render the client-safe Msg for service-layer
	// errors, wherever they sit in the wrap chain.
	for name, msg := range map[string]string{
		"code":         response.CodeFailure.WithErr(serviceErr).Msg(),
		"codeInstance": response.CodeFailure.WithStatus(http.StatusBadRequest).WithErr(serviceErr).Msg(),
		"wrapped":      response.CodeFailure.WithErr(errors.Wrap(serviceErr, "load account")).Msg(),
	} {
		if msg != "failed to load user" {
			t.Errorf("%s: msg = %q, want %q", name, msg, "failed to load user")
		}
	}

	// Plain errors keep rendering their full Error text.
	if got := response.CodeFailure.WithErr(errors.New("plain failure")).Msg(); got != "plain failure" {
		t.Errorf("plain: msg = %q, want %q", got, "plain failure")
	}
}

func TestCodeStringRendersMessageNotBareInteger(t *testing.T) {
	got := response.CodeNotFound.String()
	want := fmt.Sprintf("Requested resource not found. (code=%d)", int32(response.CodeNotFound))
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
