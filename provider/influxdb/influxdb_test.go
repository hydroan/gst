package influxdb_test

import (
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/provider/influxdb"
	"github.com/stretchr/testify/require"
)

// TestNewPassesIntervalsInMilliseconds checks the write intervals New hands
// the client: they are configured as durations, while the client counts them
// in milliseconds.
func TestNewPassesIntervalsInMilliseconds(t *testing.T) {
	cli, err := influxdb.New(config.Influxdb{
		Host:             "127.0.0.1",
		Port:             8086,
		FlushInterval:    2 * time.Second,
		RetryInterval:    3 * time.Second,
		MaxRetryInterval: 4 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(cli.Close)

	opts := cli.Options().WriteOptions()
	require.Equal(t, uint(2000), opts.FlushInterval())
	require.Equal(t, uint(3000), opts.RetryInterval())
	require.Equal(t, uint(4000), opts.MaxRetryInterval())
}
