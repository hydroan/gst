package scylla_test

import (
	"testing"

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
