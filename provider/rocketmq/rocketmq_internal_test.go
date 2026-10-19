package rocketmq

import (
	"context"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/types"
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

// TestLibraryLinesWithoutTheLoggingSetupAreDropped pins that the adapter the
// package hands the client library at load time drops a line while the
// provider's logger is unset, in a process that never ran the logging setup,
// instead of failing on it.
func TestLibraryLinesWithoutTheLoggingSetupAreDropped(t *testing.T) {
	adapter := &customLogger{l: new(types.Logger)}

	require.NotPanics(t, func() {
		adapter.Debug("sample", nil)
		adapter.Info("sample", nil)
		adapter.Warning("sample", nil)
		adapter.Error("sample", nil)
	})
}
