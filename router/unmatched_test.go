package router_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestUnmatchedRequestsAnswerInTheEnvelope pins what this server answers when a
// request reaches no handler.
//
// gin's own default is plain text carrying neither a code nor a trace id, so a
// client reading the documented envelope could not tell it from a malformed
// response — and mistyping a path is the most common way to reach it.
//
// The two ways to match nothing are answered apart: a path no route serves is
// not found, while a path that is served under another method is method not
// allowed, carrying the Allow header that names those methods.
func TestUnmatchedRequestsAnswerInTheEnvelope(t *testing.T) {
	t.Run("a path no route serves", func(t *testing.T) {
		cli, err := client.New(baseURL)
		require.NoError(t, err)

		_, err = cli.Do(http.MethodGet, "/api/there-is-no-such-route", nil)
		respErr := testutil.RequireError(t, err, http.StatusNotFound, "not found")
		requireRefusalEnvelope(t, respErr.Body, "not found")
	})

	// Read through the standard client rather than the framework one: this case
	// asserts a response header, and the framework client reports refusals as an
	// error that carries the body but not the headers.
	t.Run("a method the path does not serve", func(t *testing.T) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/-/healthz", nil)
		require.NoError(t, err)

		rsp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer rsp.Body.Close()

		body, err := io.ReadAll(rsp.Body)
		require.NoError(t, err)

		require.Equal(t, http.StatusMethodNotAllowed, rsp.StatusCode, "response body: %s", body)
		require.Equal(t, http.MethodGet, rsp.Header.Get("Allow"),
			"a method-not-allowed answer has to name the methods the path does serve")
		requireRefusalEnvelope(t, body, "method not allowed")
	})
}

// requireRefusalEnvelope asserts the fields every refusal this framework writes
// carries, whichever status it carries them under.
func requireRefusalEnvelope(t *testing.T, body []byte, msg string) {
	t.Helper()

	var envelope struct {
		Code    *int             `json:"code"`
		Msg     string           `json:"msg"`
		Data    *json.RawMessage `json:"data"`
		TraceID *string          `json:"trace_id"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope), "response body: %s", body)
	require.NotNil(t, envelope.Code, "a refusal has to carry a code, like every other response")
	require.Equal(t, -1, *envelope.Code)
	require.Equal(t, msg, envelope.Msg)
	require.NotNil(t, envelope.TraceID)
	require.NotEmpty(t, *envelope.TraceID, "a refusal has to carry the trace that explains it")
}
