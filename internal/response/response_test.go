package response

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
