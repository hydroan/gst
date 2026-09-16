package router_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMetricsAreServedWithoutCredentials pins the contract router.Init documents
// where it mounts the endpoint: a scraper holds no account here, so nothing is
// asked of it, and what keeps the endpoint private is the deployment's network
// rather than this process.
func TestMetricsAreServedWithoutCredentials(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/metrics", nil)
	require.NoError(t, err)

	rsp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer rsp.Body.Close()

	require.Equal(t, http.StatusOK, rsp.StatusCode)
}
