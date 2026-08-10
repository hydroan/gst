package client_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

// newEnvelopeServer returns a test server answering with a gst envelope and a
// capture of the last request for assertions.
func newEnvelopeServer(t *testing.T, status int, envelope string) (*httptest.Server, *http.Request) {
	t.Helper()

	captured := new(http.Request)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*captured = *r
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, envelope)
	}))
	t.Cleanup(srv.Close)
	return srv, captured
}

func TestDoParsesSuccessEnvelope(t *testing.T) {
	srv, captured := newEnvelopeServer(t, http.StatusOK,
		`{"code":0,"msg":"success","data":{"name":"sample"},"trace_id":"trace-1"}`)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	resp, err := cli.Do(http.MethodPost, "/api/records", map[string]string{"name": "sample"},
		client.WithQuery("kind", "sample"), client.WithPage(1, 10))
	require.NoError(t, err)
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "trace-1", resp.TraceID)
	require.JSONEq(t, `{"name":"sample"}`, string(resp.Data))

	require.Equal(t, "/api/records", captured.URL.Path)
	require.Equal(t, "sample", captured.URL.Query().Get("kind"))
	require.Equal(t, "1", captured.URL.Query().Get("_page"))
	require.Equal(t, "10", captured.URL.Query().Get("_size"))
}

func TestDoReturnsStructuredErrorOnRejection(t *testing.T) {
	srv, _ := newEnvelopeServer(t, http.StatusForbidden,
		`{"code":403,"msg":"permission denied","trace_id":"trace-2"}`)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	_, err = cli.Do(http.MethodGet, "/api/records", nil)
	var respErr *client.Error
	require.True(t, errors.As(err, &respErr), "error: %v", err)
	require.Equal(t, http.StatusForbidden, respErr.StatusCode)
	require.Equal(t, 403, respErr.Code)
	require.Equal(t, "permission denied", respErr.Msg)
	require.Equal(t, "trace-2", respErr.TraceID)
	require.NotEmpty(t, respErr.Body)
}

func TestDoReturnsStructuredErrorOnBusinessCodeWith2xx(t *testing.T) {
	srv, _ := newEnvelopeServer(t, http.StatusOK, `{"code":1001,"msg":"sample failure"}`)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	_, err = cli.Do(http.MethodGet, "/api/records", nil)
	var respErr *client.Error
	require.True(t, errors.As(err, &respErr))
	require.Equal(t, http.StatusOK, respErr.StatusCode)
	require.Equal(t, 1001, respErr.Code)
}

func TestClientKeepsSessionCookieAcrossRequests(t *testing.T) {
	// The login-shaped first request sets a cookie; the second request must
	// carry it back automatically through the client's cookie jar.
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/enter" {
			http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sample-session", Path: "/"})
		}
		if cookie, err := r.Cookie("session_id"); err == nil {
			gotCookie = cookie.Value
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"msg":"success"}`)
	}))
	t.Cleanup(srv.Close)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	resp, err := cli.Do(http.MethodPost, "/api/enter", nil)
	require.NoError(t, err)
	require.NotEmpty(t, resp.Cookies)

	_, err = cli.Do(http.MethodGet, "/api/records", nil)
	require.NoError(t, err)
	require.Equal(t, "sample-session", gotCookie)
}

func TestWithHeaderMergesIntoDefaultHeaders(t *testing.T) {
	srv, captured := newEnvelopeServer(t, http.StatusOK, `{"code":0}`)

	header := http.Header{}
	header.Set("X-Forwarded-Proto", "https")
	cli, err := client.New(srv.URL, client.WithHeader(header))
	require.NoError(t, err)

	_, err = cli.Do(http.MethodGet, "/api/records", nil)
	require.NoError(t, err)
	require.Equal(t, "https", captured.Header.Get("X-Forwarded-Proto"))
	// Defaults set by New must survive a WithHeader merge.
	require.Equal(t, "application/json", captured.Header.Get("Content-Type"))
}
