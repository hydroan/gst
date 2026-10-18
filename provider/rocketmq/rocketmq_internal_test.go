package rocketmq

import (
	"context"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestStartReportsAnUnreachableNameServer pins that start confirms the name
// servers answer before it reports success, instead of handing out clients
// whose first send is what fails: the client library starts a producer
// without contacting anything. The instances the bootstrap started are
// stopped first and started again afterwards, because the library keeps one
// connection per client and pins it to the name servers it first saw.
func TestStartReportsAnUnreachableNameServer(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, stop(ctx))
	saved := config.App.RocketMQ
	t.Cleanup(func() {
		config.App.RocketMQ = saved
		require.NoError(t, start(ctx))
	})
	config.App.RocketMQ.NameServerAddrs = []string{"127.0.0.1:1"}

	require.ErrorContains(t, start(ctx), "failed to connect to rocketmq")
	mu.RLock()
	defer mu.RUnlock()
	require.Nil(t, defaultProducer)
	require.Nil(t, defaultAdmin)
}
