package client_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
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

// TestWithDebugLogsTheRequestAndTheResponseThroughTheGivenLogger pins the
// debug option together with the logger option: with both, the client
// writes the request it sends and the response it gets, body included, as
// debug entries of the logger it was given.
func TestWithDebugLogsTheRequestAndTheResponseThroughTheGivenLogger(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg":"ok","data":{"id":"s-1"}}`))
	}))
	srv.Start()

	dir := t.TempDir()
	original := config.App
	config.App = new(config.Config)
	config.App.Logger.Output = config.LoggerOutputFile
	config.App.Logger.Dir = dir
	config.App.Logger.File = "client.log"
	config.App.Logger.Level = "debug"
	config.App.Logger.Format = "json"
	config.App.Logger.MaxAge = 30
	config.App.Logger.MaxSize = 100
	config.App.Logger.MaxBackups = 1
	t.Cleanup(func() { config.App = original })
	t.Cleanup(logger.Clean)

	cli, err := client.New(srv.URL, client.WithDebug(), client.WithLogger(logger.New("client.log")))
	require.NoError(t, err)
	_, err = cli.Do(t.Context(), http.MethodGet, "/api/samples/s-1", nil)
	require.NoError(t, err)
	logger.Clean()

	content, err := os.ReadFile(filepath.Join(dir, "client.log"))
	require.NoError(t, err)
	require.Contains(t, string(content), "GET /api/samples/s-1 HTTP/1.1", "the request is dumped")
	require.Contains(t, string(content), "HTTP/1.1 200 OK", "the response is dumped")
	require.Contains(t, string(content), `\"id\":\"s-1\"`, "with its body")
	require.Contains(t, string(content), `"level":"DEBUG"`)
}
