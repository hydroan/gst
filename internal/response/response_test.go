package response_test

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	ginjson "github.com/gin-gonic/gin/codec/json"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/testutil/swap"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// TestJSONEncodesWithStandardLibrary pins the success envelope — msg, data
// and trace_id, its three fields — and its encoding through encoding/json
// whatever JSON codec gin was built with. The codecs the jsoniter and go_json
// build tags select ignore omitzero, so an envelope encoded through gin's
// codec grows keys, such as a zero created_at, that the framework's models
// declare absent. The body is what json.Marshal writes, byte for byte: the
// characters HTML reads are escaped, and a nil slice and a nil map are null.
func TestJSONEncodesWithStandardLibrary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	swap.Value(t, &ginjson.API, ginjson.Core(swappedGinCodec{}))

	type sample struct {
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at,omitzero"`
	}
	type collections struct {
		Items []string          `json:"items"`
		Tags  map[string]string `json:"tags"`
	}
	tests := []struct {
		name string
		data []any
		want string
	}{
		{"with data", []any{&sample{Name: "sample"}}, `{"data":{"name":"sample"},"msg":"success","trace_id":"trace-sample"}`},
		{"without data", nil, `{"data":null,"msg":"success","trace_id":"trace-sample"}`},
		{"html characters", []any{&sample{Name: "<a&b>"}}, `{"data":{"name":"\u003ca\u0026b\u003e"},"msg":"success","trace_id":"trace-sample"}`},
		{"nil slice and map", []any{&collections{}}, `{"data":{"items":null,"tags":null},"msg":"success","trace_id":"trace-sample"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(execctx.KeyTraceID, "trace-sample")

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
	serviceErr := types.NewErrorWithCause(http.StatusInternalServerError, "failed to load user", cause)
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
		{"forbidden", types.NewError(http.StatusForbidden, "account disabled"), http.StatusForbidden, `{"data":null,"msg":"account disabled","trace_id":""}`, ""},
		{"other error", internal, http.StatusInternalServerError, `{"data":null,"msg":"The server could not process the request.","trace_id":""}`, internal.Error()},
		{"typed nil service error", (*types.Error)(nil), http.StatusInternalServerError, `{"data":null,"msg":"The server could not process the request.","trace_id":""}`, ""},
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

// TestAbortAnswersTheErrorAndStopsTheChain pins the refusal of code outside
// the controller path: a service error, the form a refusal with a status and
// message of its own takes, answers with its status and message, any other
// error with the server's own failure, the way Error answers them, with the
// handler chain aborted.
func TestAbortAnswersTheErrorAndStopsTheChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, tt := range map[string]struct {
		err    error
		status int
		msg    string
	}{
		"a service error":          {types.NewError(http.StatusForbidden, "permission denied"), http.StatusForbidden, "permission denied"},
		"the server's own failure": {errors.New("store down"), http.StatusInternalServerError, types.FailureMsg},
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(execctx.KeyTraceID, "trace-sample")

			response.Abort(c, tt.err)

			require.Equal(t, tt.status, w.Code)
			require.JSONEq(t, `{"data":null,"msg":"`+tt.msg+`","trace_id":"trace-sample"}`, w.Body.String())
			require.True(t, c.IsAborted())
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

	response.JSON(c)

	if got := w.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/problem+json")
	}
}

// TestAbortNeverAnswersWithASuccessStatus pins that a refusal from outside
// the controller path cannot leave with a status a client reads as success:
// a status that is not a failure is answered the way NewError normalizes it,
// 500 with the status text, never the status given.
func TestAbortNeverAnswersWithASuccessStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []int{http.StatusOK, http.StatusNoContent, http.StatusFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(execctx.KeyTraceID, "trace-sample")

			response.Abort(c, types.NewError(status, "refused"))

			require.Equal(t, http.StatusInternalServerError, w.Code)
			require.JSONEq(t, `{"data":null,"msg":"Internal Server Error","trace_id":"trace-sample"}`, w.Body.String())
			require.True(t, c.IsAborted())
		})
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

// TestJSONAnswersAnEncodingFailureAsTheServersOwn pins what a data no JSON
// holds, a NaN here, is answered with: the server's own failure, 500 with
// the fixed message in the failure envelope, the error recorded on the
// context for the access log and the handler chain stopped, and not the 200
// with an empty body gin's render leaves behind when the encoding fails
// under it.
//
// The failure envelope is the whole body wherever the encoding failed: a
// value that fails after a part of the data was encoded leaves nothing of
// that part in it.
func TestJSONAnswersAnEncodingFailureAsTheServersOwn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		data any
	}{
		{"nothing encoded before the failure", map[string]any{"ratio": math.NaN()}},
		{"a part of the data encoded before the failure", []any{strings.Repeat("x", 100_000), math.NaN()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(execctx.KeyTraceID, "trace-sample")

			response.JSON(c, tt.data)

			require.Equal(t, http.StatusInternalServerError, w.Code)
			require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
			require.JSONEq(t, `{"data":null,"msg":"The server could not process the request.","trace_id":"trace-sample"}`, w.Body.String())
			require.Len(t, c.Errors, 1)
			require.True(t, c.IsAborted())
		})
	}
}

// TestJSONWritesEachBodyWhole pins the body of a response to its own data
// whatever the responses before it and beside it carried: the envelopes are
// encoded into buffers that are used again, so a body must hold nothing of
// another response, after a larger one, after one too large to keep its
// buffer, and among many written at once.
func TestJSONWritesEachBodyWhole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	write := func(data string) (status int, body string) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(execctx.KeyTraceID, "trace-sample")

		response.JSON(c, data)

		return w.Code, w.Body.String()
	}

	t.Run("one after another", func(t *testing.T) {
		for _, size := range []int{100_000, 10, 70_000, 0, 5_000, 1} {
			data := strings.Repeat("x", size)
			status, body := write(data)
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, `{"data":"`+data+`","msg":"success","trace_id":"trace-sample"}`, body)
		}
	})

	t.Run("many at once", func(t *testing.T) {
		var wg sync.WaitGroup
		for worker := range 8 {
			wg.Go(func() {
				for round := range 200 {
					data := strings.Repeat(strconv.Itoa(worker), 1+(worker*977+round*131)%6_000)
					status, body := write(data)
					if status != http.StatusOK || body != `{"data":"`+data+`","msg":"success","trace_id":"trace-sample"}` {
						t.Errorf("worker %d round %d: status %d, and the body, %d bytes, is not the response's own", worker, round, status, len(body))
						return
					}
				}
			})
		}
		wg.Wait()
	})
}

// BenchmarkJSON measures writing a success envelope, the answer of every
// successful request: one record, and a page of twenty, the answer of a
// list.
func BenchmarkJSON(b *testing.B) {
	gin.SetMode(gin.TestMode)
	type sample struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Remark    string    `json:"remark"`
		Amount    int64     `json:"amount"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	type page struct {
		Items []*sample `json:"items"`
		Total int64     `json:"total"`
	}
	at := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	items := make([]*sample, 20)
	for i := range items {
		items[i] = &sample{
			ID:        "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f40" + strconv.Itoa(10+i),
			Name:      "sample " + strconv.Itoa(i),
			Remark:    "a remark as long as the ones a record usually carries",
			Amount:    int64(i) * 100,
			CreatedAt: at,
			UpdatedAt: at,
		}
	}
	cases := []struct {
		name string
		data any
	}{
		{"record", items[0]},
		{"page", &page{Items: items, Total: 100_000}},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			w := httptest.NewRecorder()

			b.ReportAllocs()
			for b.Loop() {
				w.Body.Reset()
				c, _ := gin.CreateTestContext(w)
				c.Set(execctx.KeyTraceID, "trace-sample")
				response.JSON(c, tc.data)
			}
		})
	}
}
