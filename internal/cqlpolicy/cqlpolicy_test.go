package cqlpolicy_test

import (
	"context"
	"testing"

	"github.com/gocql/gocql"
	"github.com/hydroan/gst/internal/cqlpolicy"
	"github.com/stretchr/testify/require"
)

// TestDowngradingConsistency pins the levels DowngradingConsistency lists
// for each level a session may run at, the examples on it among them, and
// that gocql retries a query once under the policy, at the listed level.
func TestDowngradingConsistency(t *testing.T) {
	for _, tc := range []struct {
		level gocql.Consistency
		want  []gocql.Consistency
	}{
		{gocql.All, []gocql.Consistency{gocql.One}},
		{gocql.EachQuorum, []gocql.Consistency{gocql.One}},
		{gocql.Quorum, []gocql.Consistency{gocql.One}},
		{gocql.Three, []gocql.Consistency{gocql.One}},
		{gocql.Two, []gocql.Consistency{gocql.One}},
		{gocql.LocalQuorum, []gocql.Consistency{gocql.LocalOne}},
		{gocql.One, nil},
		{gocql.LocalOne, nil},
		{gocql.Any, nil},
	} {
		require.Equal(t, tc.want, cqlpolicy.DowngradingConsistency(tc.level).ConsistencyLevelsToTry, tc.level.String())
	}

	policy := cqlpolicy.DowngradingConsistency(gocql.Quorum)
	failedOnce := &retriedQuery{attempts: 1, consistency: gocql.Quorum}
	require.True(t, policy.Attempt(failedOnce))
	require.Equal(t, gocql.One, failedOnce.consistency)
	require.False(t, policy.Attempt(&retriedQuery{attempts: 2, consistency: gocql.One}))
}

// TestReconnectAttempts pins the examples on ReconnectAttempts.
func TestReconnectAttempts(t *testing.T) {
	require.Equal(t, 1, cqlpolicy.ReconnectAttempts(0))
	require.Equal(t, 4, cqlpolicy.ReconnectAttempts(3))
}

// retriedQuery is a query gocql has tried attempts times, the last time at
// consistency.
type retriedQuery struct {
	attempts    int
	consistency gocql.Consistency
}

func (q *retriedQuery) Attempts() int                      { return q.attempts }
func (q *retriedQuery) SetConsistency(c gocql.Consistency) { q.consistency = c }
func (q *retriedQuery) GetConsistency() gocql.Consistency  { return q.consistency }
func (q *retriedQuery) Context() context.Context           { return context.Background() }
