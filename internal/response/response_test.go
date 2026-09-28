package response_test

import (
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
	"github.com/stretchr/testify/require"
)

// TestJSONEncodesWithStandardLibrary pins the success envelope — its three
// fields, no code among them — and its encoding through encoding/json
// whatever JSON codec gin was built with. The codecs the jsoniter and go_json
// build tags select ignore omitzero, so an envelope encoded through gin's
// codec grows keys, such as a zero created_at, that the framework's models
// declare absent.
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
		{"with data", []any{&sample{Name: "sample"}}, `{"data":{"name":"sample"},"msg":"success","trace_id":"trace-sample"}`},
		{"without data", nil, `{"data":null,"msg":"success","trace_id":"trace-sample"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(consts.TRACE_ID, "trace-sample")

			response.JSON(c, tt.data...)

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

// TestErrorAnswersServiceErrorsAndHidesTheRest pins the failure envelope: a
// service error, wherever it sits in the wrap chain, answers with the status
// and client-safe message it was constructed with, its cause kept out of the
// body; any other error is the server's own failure and answers 500 with the
// fixed message, its text kept out of the body as well.
func TestErrorAnswersServiceErrorsAndHidesTheRest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cause := errors.New("database password leaked")
	serviceErr := serviceregistry.NewErrorWithCause(http.StatusInternalServerError, "failed to load user", cause)
	internal := errors.New("dial tcp 10.0.0.1:3306: connection refused")

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
		hidden     string
	}{
		{"service error", serviceErr, http.StatusInternalServerError, `{"data":null,"msg":"failed to load user","trace_id":""}`, cause.Error()},
		{"wrapped service error", errors.Wrap(serviceErr, "load account"), http.StatusInternalServerError, `{"data":null,"msg":"failed to load user","trace_id":""}`, cause.Error()},
		{"forbidden", serviceregistry.NewError(http.StatusForbidden, "account disabled"), http.StatusForbidden, `{"data":null,"msg":"account disabled","trace_id":""}`, ""},
		{"other error", internal, http.StatusInternalServerError, `{"data":null,"msg":"The server could not process the request.","trace_id":""}`, internal.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			response.Error(c, tt.err)

			require.Equal(t, tt.wantStatus, w.Code)
			require.JSONEq(t, tt.wantBody, w.Body.String())
			if tt.hidden != "" {
				require.NotContains(t, w.Body.String(), tt.hidden)
			}
		})
	}
}

// TestAbortWritesTheFailureEnvelopeAndStopsTheChain pins the refusal of
// code outside the controller path: the status and message given, in the
// failure envelope, with the handler chain aborted.
func TestAbortWritesTheFailureEnvelopeAndStopsTheChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(consts.TRACE_ID, "trace-sample")

	response.Abort(c, http.StatusForbidden, "permission denied")

	require.Equal(t, http.StatusForbidden, w.Code)
	require.JSONEq(t, `{"data":null,"msg":"permission denied","trace_id":"trace-sample"}`, w.Body.String())
	require.True(t, c.IsAborted())
}

// TestJSONKeepsContentTypeSetBeforehand pins that the envelope render, like
// gin's own JSON render, leaves a Content-Type already set on the response
// alone.
func TestJSONKeepsContentTypeSetBeforehand(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Header("Content-Type", "application/problem+json")

	response.JSON(c)

	if got := w.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/problem+json")
	}
}

// TestAbortWritesNoBodyForBodylessStatus pins the envelope on a status that
// cannot carry a body: the JSON Content-Type is still announced and nothing is
// written.
func TestAbortWritesNoBodyForBodylessStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	response.Abort(c, http.StatusNoContent, "")

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

	response.JSON(c, math.NaN())

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
