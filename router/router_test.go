package router_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Routes: func() {
			if err := router.Init(); err != nil {
				panic(err)
			}
		},
	})
}

// TestUnmatchedRequestsAnswerInTheEnvelope pins what this server answers when
// no route matches.
//
// gin's own default is plain text carrying neither a code nor a trace id, so a
// client reading the documented envelope could not tell it from a malformed
// response — and mistyping a path is the most common way to reach it.
//
// A method the path does not serve is answered the same way rather than as
// method not allowed. Nothing authenticates a caller before this point, so
// separating the two would let anyone enumerate the paths this server
// registers, one request at a time.
func TestUnmatchedRequestsAnswerInTheEnvelope(t *testing.T) {
	requireNotFoundEnvelope := func(t *testing.T, method, path string) {
		t.Helper()

		cli, err := client.New(testutil.BaseURL())
		require.NoError(t, err)

		_, err = cli.Do(method, path, nil)
		respErr := testutil.RequireError(t, err, http.StatusNotFound, "not found")

		var envelope struct {
			Code    *int             `json:"code"`
			Msg     string           `json:"msg"`
			Data    *json.RawMessage `json:"data"`
			TraceID *string          `json:"trace_id"`
		}
		require.NoError(t, json.Unmarshal(respErr.Body, &envelope),
			"response body: %s", respErr.Body)
		require.NotNil(t, envelope.Code, "a refusal has to carry a code, like every other response")
		require.Equal(t, -1, *envelope.Code)
		require.Equal(t, "not found", envelope.Msg)
		require.NotNil(t, envelope.TraceID)
		require.NotEmpty(t, *envelope.TraceID, "a refusal has to carry the trace that explains it")
	}

	t.Run("a path no route serves", func(t *testing.T) {
		requireNotFoundEnvelope(t, http.MethodGet, "/api/there-is-no-such-route")
	})

	t.Run("a method the path does not serve", func(t *testing.T) {
		requireNotFoundEnvelope(t, http.MethodPost, "/-/healthz")
	})
}
