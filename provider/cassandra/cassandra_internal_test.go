package cassandra

import (
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestNewClusterRetriesADowngradingQueryOnceAtOne pins the retry policy
// downgrading_consistency gives a session at quorum: one retry, at one.
func TestNewClusterRetriesADowngradingQueryOnceAtOne(t *testing.T) {
	cluster, err := newCluster(config.Cassandra{
		Hosts:       []string{"127.0.0.1"},
		Consistency: config.ConsistencyQuorum,
		RetryPolicy: config.RetryPolicyDowngradingConsistency,
	})
	require.NoError(t, err)
	policy, ok := cluster.RetryPolicy.(*gocql.DowngradingConsistencyRetryPolicy)
	require.True(t, ok, "retry policy %T", cluster.RetryPolicy)
	require.Equal(t, []gocql.Consistency{gocql.One}, policy.ConsistencyLevelsToTry)
}

// TestNewClusterConnectsOnceMoreThanItRetries pins that max_retry_count
// counts the retries of a connection, the first try aside: 0 still connects
// once, which a reconnection policy allowed no try at all never does.
func TestNewClusterConnectsOnceMoreThanItRetries(t *testing.T) {
	for _, tc := range []struct{ retries, tries int }{{0, 1}, {3, 4}} {
		cluster, err := newCluster(config.Cassandra{
			Hosts:             []string{"127.0.0.1"},
			ReconnectInterval: time.Second,
			MaxRetryCount:     tc.retries,
		})
		require.NoError(t, err)
		require.Equal(t, tc.tries, cluster.ReconnectionPolicy.GetMaxRetries(), "max_retry_count %d", tc.retries)
	}
}
