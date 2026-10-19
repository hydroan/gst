package testcontainer

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestSetupRocketMQ(t *testing.T) {
	t.Run("on the port the host picks", func(t *testing.T) {
		setupRocketMQAndSend(t)
	})

	// The broker derives its other ports from the one it listens on unless
	// the configuration pins them; the highest port leaves no room above it
	// for the HA port, see rocketmqBrokerConf.
	t.Run("on the highest port", func(t *testing.T) {
		const highest = "127.0.0.1:65535"
		probe, err := net.Listen("tcp", highest)
		if err != nil {
			t.Skipf("%s is taken on this host: %v", highest, err)
		}
		require.NoError(t, probe.Close())
		saved := rocketmqBrokerListenAddress
		rocketmqBrokerListenAddress = highest
		t.Cleanup(func() { rocketmqBrokerListenAddress = saved })

		setupRocketMQAndSend(t)
	})
}

// setupRocketMQAndSend starts the container and sends a message through it.
// A send is what proves the address the broker announces points back at the
// published port: the client learns it from the name server and dials the
// broker itself; a bare dial of the name server would pass on a broker no
// client could reach.
func setupRocketMQAndSend(t *testing.T) {
	t.Helper()
	isolateEnv(t, config.ROCKETMQ_NAME_SERVER_ADDRS, config.ROCKETMQ_ENABLED)

	cleanup, err := SetupRocketMQ()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })

	addr := os.Getenv(config.ROCKETMQ_NAME_SERVER_ADDRS)
	require.NotEmpty(t, addr)
	require.Equal(t, "true", os.Getenv(config.ROCKETMQ_ENABLED))

	p, err := producer.NewDefaultProducer(
		producer.WithNameServer([]string{addr}),
		producer.WithGroupName("sample-group"),
	)
	require.NoError(t, err)
	require.NoError(t, p.Start())
	defer func() { require.NoError(t, p.Shutdown()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := p.SendSync(ctx, primitive.NewMessage("sample", []byte("value")))
	require.NoError(t, err)
	require.Equal(t, primitive.SendOK, result.Status)
}
