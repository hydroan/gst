// Package cqlpolicy builds the gocql policies the cassandra and scylla
// providers configure from settings both of them carry, so that a setting
// means the same in either: the retry policy downgrading_consistency names
// and the tries a reconnection policy makes.
package cqlpolicy

import "github.com/gocql/gocql"

// DowngradingConsistency returns the retry policy downgrading_consistency
// names for a session at level: a query the replicas fail to serve at level
// is retried once, at one, or at local_one for local_quorum, which keeps the
// retry in the local datacenter; at one, local_one or any, the weakest
// levels, it is not retried. For quorum the policy lists [ONE], for
// local_quorum [LOCAL_ONE] and for one nothing. gocql derives no list of its
// own: its policy retries a query the n-th time at the n-th level the list
// holds and stops once the list runs out, so the list is every retry there
// is. One retry is what the DataStax drivers' downgrading policies make.
func DowngradingConsistency(level gocql.Consistency) *gocql.DowngradingConsistencyRetryPolicy {
	switch level {
	case gocql.Any, gocql.One, gocql.LocalOne:
		return &gocql.DowngradingConsistencyRetryPolicy{}
	case gocql.LocalQuorum:
		return &gocql.DowngradingConsistencyRetryPolicy{ConsistencyLevelsToTry: []gocql.Consistency{gocql.LocalOne}}
	default:
		return &gocql.DowngradingConsistencyRetryPolicy{ConsistencyLevelsToTry: []gocql.Consistency{gocql.One}}
	}
}

// ReconnectAttempts returns how many tries a reconnection policy makes to
// connect to a host for a setting allowing retries retries: the first try
// and the retries, 1 for 0 and 4 for 3. gocql counts the tries in a
// reconnection policy's MaxRetries, not the retries: at 0 it never tries,
// and the pool goes on with a connection that is nil.
func ReconnectAttempts(retries int) int {
	return retries + 1
}
