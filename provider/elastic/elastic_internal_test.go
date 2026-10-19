package elastic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestIndexExistsReportsAnAnswerItDoesNotKnow pins that Exists answers false
// only when the cluster says the index is not found: a refusal, here
// forbidden, is an error rather than an index that is not there.
func TestIndexExistsReportsAnAnswerItDoesNotKnow(t *testing.T) {
	useCluster(t, answering(t, http.StatusForbidden))

	exists, err := Index.Exists(context.Background(), "sample")
	require.ErrorContains(t, err, "403")
	require.False(t, exists)
}

// TestStartReportsAClusterThatRefusesThePing pins that start confirms the
// cluster answers the ping before it reports a connection: a cluster that
// refuses the credentials answers it, with 401.
func TestStartReportsAClusterThatRefusesThePing(t *testing.T) {
	saved, savedConfig := client, config.App.Elasticsearch
	t.Cleanup(func() { client, config.App.Elasticsearch = saved, savedConfig })
	config.App.Elasticsearch.Addrs = []string{answering(t, http.StatusUnauthorized)}

	require.ErrorContains(t, start(context.Background()), "401")
}

// answering starts a server answering every request with status and returns
// its address.
func answering(t *testing.T, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// useCluster points the package client at addr until the test ends.
func useCluster(t *testing.T, addr string) {
	t.Helper()
	c, err := New(config.Elasticsearch{Addrs: []string{addr}, DisableRetries: true})
	require.NoError(t, err)
	saved := client
	client = c
	t.Cleanup(func() { client = saved })
}
