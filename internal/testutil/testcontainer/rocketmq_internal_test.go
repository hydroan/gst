package testcontainer

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestSetupRocketMQ(t *testing.T) {
	isolateEnv(t, config.ROCKETMQ_NAME_SERVER_ADDRS, config.ROCKETMQ_ENABLED)

	cleanup, err := SetupRocketMQ()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })

	addr := os.Getenv(config.ROCKETMQ_NAME_SERVER_ADDRS)
	require.NotEmpty(t, addr)
	require.Equal(t, "true", os.Getenv(config.ROCKETMQ_ENABLED))

	// A send is what proves the address the broker announces points back at
	// the published port: the client learns it from the name server and
	// dials the broker itself; a bare dial of the name server would pass on
	// a broker no client could reach.
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
