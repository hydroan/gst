package response

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	ginjson "github.com/gin-gonic/gin/codec/json"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/serviceregistry"
)

// TestJSONEncodesWithStandardLibrary pins the response envelope to
// encoding/json whatever JSON codec gin was built with. The codecs the
// jsoniter and go_json build tags select ignore omitzero, so an envelope
// encoded through gin's codec grows keys, such as a zero created_at, that the
// framework's models declare absent.
func TestJSONEncodesWithStandardLibrary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restore := ginjson.API
	ginjson.API = swappedGinCodec{}
	t.Cleanup(func() { ginjson.API = restore })

	type sample struct {
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at,omitzero"`
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(consts.TRACE_ID, "trace-sample")

	JSON(c, CodeSuccess, &sample{Name: "sample"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json; charset=utf-8")
	}
	want := `{"code":0,"data":{"name":"sample"},"msg":"success","trace_id":"trace-sample"}`
	if got := w.Body.String(); got != want {
		t.Errorf("body = %s, want %s", got, want)
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

	Attachment(c, []byte("hello"), "exported.csv", "text/csv; charset=utf-8")

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
		"code":         CodeFailure.WithErr(serviceErr).Msg(),
		"codeInstance": CodeFailure.WithStatus(http.StatusBadRequest).WithErr(serviceErr).Msg(),
		"wrapped":      CodeFailure.WithErr(errors.Wrap(serviceErr, "load account")).Msg(),
	} {
		if msg != "failed to load user" {
			t.Errorf("%s: msg = %q, want %q", name, msg, "failed to load user")
		}
	}

	// Plain errors keep rendering their full Error text.
	if got := CodeFailure.WithErr(errors.New("plain failure")).Msg(); got != "plain failure" {
		t.Errorf("plain: msg = %q, want %q", got, "plain failure")
	}
}

func TestCodeStringRendersMessageNotBareInteger(t *testing.T) {
	got := CodeNotFound.String()
	want := fmt.Sprintf("Requested resource not found. (code=%d)", int32(CodeNotFound))
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
