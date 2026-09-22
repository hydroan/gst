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

			_, err = cli.Do(http.MethodGet, "/api/records", nil)
			var netErr net.Error
			require.True(t, errors.As(err, &netErr) && netErr.Timeout(), "want a timeout, got %v", err)
			require.Zero(t, shared.Timeout, "the shared client keeps its own timeout")
		})
	}
}
