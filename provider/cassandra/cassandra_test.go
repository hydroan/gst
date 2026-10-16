package cassandra_test

import (
	"testing"

	"github.com/gocql/gocql"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/provider/cassandra"
	"github.com/stretchr/testify/require"
)

// TestNewReportsAPolicyItDoesNotKnow pins that a consistency or retry policy
// outside its set is an error from New, before any host is dialed, instead
// of a setting left at the driver's default. The configuration decode does
// not let such a value through; one set in code arrives here.
func TestNewReportsAPolicyItDoesNotKnow(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  config.Cassandra
		want string
	}{
		{name: "consistency", cfg: config.Cassandra{Consistency: "most"}, want: "consistency: most"},
		{name: "retry_policy", cfg: config.Cassandra{RetryPolicy: "fallthrough"}, want: "retry policy: fallthrough"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cassandra.New(tt.cfg)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

// TestNewReadsTheConsistencyInAnyLetterCase pins that a consistency set in
// code is read however it is cased, as the driver itself prints the names in
// upper case: with the name accepted, New gets as far as the session, which
// reports the missing hosts.
func TestNewReadsTheConsistencyInAnyLetterCase(t *testing.T) {
	for _, level := range []config.Consistency{"QUORUM", "Local_Quorum", config.ConsistencyLocalOne} {
		_, err := cassandra.New(config.Cassandra{Consistency: level})
		require.ErrorIs(t, err, gocql.ErrNoHosts, string(level))
	}
}
