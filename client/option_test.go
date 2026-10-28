package client_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

// TestWithBasicAuthSendsTheCredentialsOfANamedUser pins that WithBasicAuth
// sends basic credentials on every request, the username trimmed of the
// spaces around it, and sends none for a username that is blank.
func TestWithBasicAuthSendsTheCredentialsOfANamedUser(t *testing.T) {
	type seen struct {
		username, password, header string
		ok                         bool
	}
	got := make(chan seen, 1)
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		got <- seen{username: username, password: password, header: r.Header.Get("Authorization"), ok: ok}
		w.WriteHeader(http.StatusNoContent)
	}))
	srv.Start()

	for _, tt := range []struct {
		name               string
		username, password string
		want               seen
	}{
		{name: "named", username: " alice ", password: "secret", want: seen{username: "alice", password: "secret", header: "Basic YWxpY2U6c2VjcmV0", ok: true}},
		{name: "blank", username: "  ", password: "secret", want: seen{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := client.New(srv.URL, client.WithBasicAuth(tt.username, tt.password))
			require.NoError(t, err)

			_, _ = cli.Do(t.Context(), http.MethodGet, "/api/records", nil)
			select {
			case request := <-got:
				require.Equal(t, tt.want, request)
			case <-time.After(5 * time.Second):
				t.Fatal("the request never reached the server")
			}
		})
	}
}

// TestWithTimeoutLeavesTheGivenHTTPClientAlone gives the client a shared
// http.Client and a timeout, in either order: every request times out, and
// the shared client keeps its own settings.
func TestWithTimeoutLeavesTheGivenHTTPClientAlone(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	srv.Start()

	const timeout = 50 * time.Millisecond
	tests := []struct {
		name string
		opts func(shared *http.Client) []client.Option
	}{
		{
			name: "timeout after the http client",
			opts: func(shared *http.Client) []client.Option {
				return []client.Option{client.WithHTTPClient(shared), client.WithTimeout(timeout)}
			},
		},
		{
			name: "timeout before the http client",
			opts: func(shared *http.Client) []client.Option {
				return []client.Option{client.WithTimeout(timeout), client.WithHTTPClient(shared)}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shared := &http.Client{}
			cli, err := client.New(srv.URL, tt.opts(shared)...)
			require.NoError(t, err)

			_, err = cli.Do(t.Context(), http.MethodGet, "/api/records", nil)
			var netErr net.Error
			require.True(t, errors.As(err, &netErr) && netErr.Timeout(), "want a timeout, got %v", err)
			require.Zero(t, shared.Timeout, "the shared client keeps its own timeout")
		})
	}
}
