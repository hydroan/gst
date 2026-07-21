package response

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/serviceregistry"
)

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
