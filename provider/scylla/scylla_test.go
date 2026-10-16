package scylla_test

import (
	"testing"

	"github.com/gocql/gocql"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/provider/scylla"
	"github.com/stretchr/testify/require"
)

// TestNewReportsAPolicyItDoesNotKnow pins that a consistency, retry policy or
// reconnect policy outside its set is an error from New, before any host is
// dialed, instead of a setting left at the driver's default. The
// configuration decode does not let such a value through; one set in code
// arrives here.
func TestNewReportsAPolicyItDoesNotKnow(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  config.Scylla
		want string
	}{
		{name: "consistency", cfg: config.Scylla{Consistency: "most"}, want: "consistency: most"},
		{name: "retry_policy", cfg: config.Scylla{RetryPolicy: "never"}, want: "retry policy: never"},
		{name: "reconnect_policy", cfg: config.Scylla{ReconnectPolicy: "linear"}, want: "reconnect policy: linear"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := scylla.New(tt.cfg)
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
		_, err := scylla.New(config.Scylla{Consistency: level})
		require.ErrorIs(t, err, gocql.ErrNoHosts, string(level))
	}
}

// TestEveryConfiguredConsistencyIsADriverLevel pins that each consistency
// level the configuration admits is a name the driver parses: a level added
// to config under a spelling the driver does not know would be an error from
// New although the configuration accepted it.
func TestEveryConfiguredConsistencyIsADriverLevel(t *testing.T) {
	for _, level := range []config.Consistency{
		config.ConsistencyAny, config.ConsistencyOne, config.ConsistencyTwo, config.ConsistencyThree,
		config.ConsistencyQuorum, config.ConsistencyAll, config.ConsistencyLocalQuorum,
		config.ConsistencyEachQuorum, config.ConsistencyLocalOne,
	} {
		_, err := gocql.ParseConsistencyWrapper(string(level))
		require.NoError(t, err, string(level))
	}
}
