package testcontainer

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestSetupElasticsearch(t *testing.T) {
	isolateEnv(t, config.ELASTICSEARCH_ADDRS, config.ELASTICSEARCH_ENABLED)

	cleanup, err := SetupElasticsearch()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })

	addr := os.Getenv(config.ELASTICSEARCH_ADDRS)
	require.NotEmpty(t, addr)
	require.Equal(t, "true", os.Getenv(config.ELASTICSEARCH_ENABLED))

	// The cluster answers plain HTTP without credentials, which is what the
	// framework is pointed at.
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Get(addr + "/_cluster/health")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var health struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&health))
	require.Contains(t, []string{"green", "yellow"}, health.Status)
}
