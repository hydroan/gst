package testutil

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestServerPreparesClickhouse(t *testing.T) {
	for _, key := range []string{config.CLICKHOUSE_HOST, config.CLICKHOUSE_PORT, config.CLICKHOUSE_ENABLED} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}

	release, _, err := Server{Clickhouse: true}.prepare()
	t.Cleanup(release)
	require.NoError(t, err)

	// The prepared instance lands in the environment the way the other
	// services do, which is where the bootstrap config picks it up.
	host := os.Getenv(config.CLICKHOUSE_HOST)
	port := os.Getenv(config.CLICKHOUSE_PORT)
	require.NotEmpty(t, host)
	require.NotEmpty(t, port)
	require.Equal(t, "true", os.Getenv(config.CLICKHOUSE_ENABLED))

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func TestServerPreparesMinio(t *testing.T) {
	for _, key := range []string{config.MINIO_ENDPOINT, config.MINIO_BUCKET, config.MINIO_ENABLED} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}

	release, _, err := Server{Minio: true}.prepare()
	t.Cleanup(release)
	require.NoError(t, err)

	// The prepared bucket lands in the environment the way the other
	// services do, which is where the bootstrap config picks it up.
	endpoint := os.Getenv(config.MINIO_ENDPOINT)
	require.NotEmpty(t, endpoint)
	require.NotEmpty(t, os.Getenv(config.MINIO_BUCKET))
	require.Equal(t, "true", os.Getenv(config.MINIO_ENABLED))

	conn, err := net.DialTimeout("tcp", endpoint, 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

// TestServerPreparesTheServicesWithAContainerPerBinary pins that prepare runs
// the setup of each service whose container every test binary starts afresh,
// once, and that the release it returns releases what the setup started. The
// setups are stand-ins: what the real ones start and publish through the
// environment is pinned against real containers by TestSetupKafka,
// TestSetupRocketMQ and TestSetupElasticsearch in the testcontainer package.
func TestServerPreparesTheServicesWithAContainerPerBinary(t *testing.T) {
	for _, tt := range []struct {
		name   string
		server Server
		setup  *func() (func() error, error)
	}{
		{"kafka", Server{Kafka: true}, &setupKafka},
		{"rocketmq", Server{RocketMQ: true}, &setupRocketMQ},
		{"elasticsearch", Server{Elasticsearch: true}, &setupElasticsearch},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setup := stubSetup(t, tt.setup)

			release, _, err := tt.server.prepare()
			t.Cleanup(func() {
				release()
				if setup.released != 1 {
					t.Errorf("the release prepare returned released the service %d times, want once", setup.released)
				}
			})
			require.NoError(t, err)
			require.Equal(t, 1, setup.started)
			require.Zero(t, setup.released, "prepare released the service before the tests ran")
		})
	}
}

// setupStub stands in for the setup of a service prepare runs, counting the
// starts and the releases of the service it pretends to start.
type setupStub struct {
	started, released int
}

// stubSetup replaces the setup *setup with a setupStub until the test ends.
func stubSetup(t *testing.T, setup *func() (func() error, error)) *setupStub {
	t.Helper()

	stub := &setupStub{}
	saved := *setup
	t.Cleanup(func() { *setup = saved })
	*setup = func() (func() error, error) {
		stub.started++
		return func() error {
			stub.released++
			return nil
		}, nil
	}
	return stub
}
