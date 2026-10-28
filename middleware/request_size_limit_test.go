package middleware_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/middleware"
	"github.com/stretchr/testify/require"
)

// sizeLimitUnderTest is the body limit the RequestSizeLimit tests mount.
const sizeLimitUnderTest = 8

// TestRequestSizeLimitRefusesADeclaredLengthOverTheLimit pins the early
// refusal: a body whose Content-Length is over the limit is answered 400
// before the handler runs, while one of exactly the limit goes through whole.
func TestRequestSizeLimitRefusesADeclaredLengthOverTheLimit(t *testing.T) {
	w := serveSizeLimited(t, strings.Repeat("x", sizeLimitUnderTest+1), true)
	require.Equal(t, http.StatusBadRequest, w.Code)
	var envelope struct {
		Msg string `json:"msg"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope), w.Body.String())
	require.Equal(t, "request body too large", envelope.Msg)

	w = serveSizeLimited(t, strings.Repeat("x", sizeLimitUnderTest), true)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "read "+strings.Repeat("x", sizeLimitUnderTest), w.Body.String())
}

// TestRequestSizeLimitCutsABodyOfUnknownLength pins the limit on a body
// that declares no length, a chunked one: the request reaches the handler,
// whose read of the body fails at the limit with *http.MaxBytesError.
func TestRequestSizeLimitCutsABodyOfUnknownLength(t *testing.T) {
	w := serveSizeLimited(t, strings.Repeat("x", sizeLimitUnderTest+1), false)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "cut at 8", w.Body.String())

	w = serveSizeLimited(t, strings.Repeat("x", sizeLimitUnderTest), false)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "read "+strings.Repeat("x", sizeLimitUnderTest), w.Body.String())
}

// TestRequestSizeLimitPassesARequestWithoutBody pins that a request without
// a body goes through untouched.
func TestRequestSizeLimitPassesARequestWithoutBody(t *testing.T) {
	router := newSizeLimitedRouter(t)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/upload", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "read ", w.Body.String())
}

// serveSizeLimited posts body through RequestSizeLimit, with its length
// declared in Content-Length or, when declared is false, unknown as a
// chunked body's is.
func serveSizeLimited(t *testing.T, body string, declared bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/upload", strings.NewReader(body))
	if !declared {
		req.ContentLength = -1
	}
	w := httptest.NewRecorder()
	newSizeLimitedRouter(t).ServeHTTP(w, req)
	return w
}

// newSizeLimitedRouter mounts RequestSizeLimit in front of a handler that
// reads the whole body and answers with what it read, or with the limit its
// read was cut at.
func newSizeLimitedRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestSizeLimit(sizeLimitUnderTest))
	router.POST("/api/upload", func(c *gin.Context) {
		data, err := io.ReadAll(c.Request.Body)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.String(http.StatusOK, "cut at %d", tooLarge.Limit)
			return
		}
		require.NoError(t, err)
		c.String(http.StatusOK, "read %s", data)
	})
	return router
}
