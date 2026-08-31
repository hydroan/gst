package router_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestProbesAnswerOnARunningServer walks the probe endpoints over a real
// server, which is what pins them to the routes router.Init registers: both
// answer OK on a process that is serving and not shutting down.
func TestProbesAnswerOnARunningServer(t *testing.T) {
	for _, probe := range []string{"/-/healthz", "/-/readyz"} {
		t.Run(probe, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+probe, nil)
			require.NoError(t, err)

			rsp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer rsp.Body.Close()

			require.Equal(t, http.StatusOK, rsp.StatusCode)
		})
	}
}
