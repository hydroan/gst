package scylla

import (
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestNewClusterRetriesADowngradingQueryOnceAtLocalOne pins the retry
// policy downgrading_consistency gives a session at local_quorum: one
// retry, at local_one, which keeps it in the local datacenter.
func TestNewClusterRetriesADowngradingQueryOnceAtLocalOne(t *testing.T) {
	cluster, err := newCluster(config.Scylla{
		Hosts:       []string{"127.0.0.1:9042"},
		Consistency: config.ConsistencyLocalQuorum,
		RetryPolicy: config.RetryPolicyDowngradingConsistency,
	})
	require.NoError(t, err)
	policy, ok := cluster.RetryPolicy.(*gocql.DowngradingConsistencyRetryPolicy)
	require.True(t, ok, "retry policy %T", cluster.RetryPolicy)
	require.Equal(t, []gocql.Consistency{gocql.LocalOne}, policy.ConsistencyLevelsToTry)
}

// TestNewClusterConnectsOnceMoreThanItRetries pins that
// reconnect_max_retries counts the retries of a connection, the first try
// aside, under either reconnect policy: 0 still connects once, which a
// reconnection policy allowed no try at all never does.
func TestNewClusterConnectsOnceMoreThanItRetries(t *testing.T) {
	for _, policy := range []config.ReconnectPolicy{config.ReconnectPolicyConstant, config.ReconnectPolicyExponential} {
		for _, tc := range []struct{ retries, tries int }{{0, 1}, {10, 11}} {
			cluster, err := newCluster(config.Scylla{
				Hosts:                     []string{"127.0.0.1:9042"},
				ReconnectPolicy:           policy,
				ReconnectMaxRetries:       tc.retries,
				ReconnectConstantInterval: time.Second,
				ReconnectInitialInterval:  100 * time.Millisecond,
				ReconnectMaxInterval:      time.Second,
			})
			require.NoError(t, err)
			require.Equal(t, tc.tries, cluster.ReconnectionPolicy.GetMaxRetries(), "%s reconnect_max_retries %d", policy, tc.retries)
		}
	}
}
