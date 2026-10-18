package testcontainer

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	rocketmqImage          = "apache/rocketmq:5.3.4"
	rocketmqNameServerPort = "9876/tcp"
	rocketmqBrokerConfPath = "/tmp/broker.conf"

	// rocketmqHeapOptions sizes both JVMs through the HEAP_OPTS variable the
	// image's start scripts read; their own defaults reserve two gigabytes
	// for the broker, every page of them touched at start, and one for the
	// name server.
	rocketmqHeapOptions = "-Xms512m -Xmx512m -Xmn256m"

	// rocketmqBrokerLog is the broker's own log inside the container, and
	// rocketmqRegisteredLine the line it writes once the name server knows
	// it. The broker prints its boot line before its registration with the
	// name server has completed; the first completed one follows some ten
	// seconds later, and until then no route resolves, so the readiness wait
	// is for this line rather than for the boot lines.
	rocketmqBrokerLog      = "/home/rocketmq/logs/rocketmqlogs/broker.log"
	rocketmqRegisteredLine = "Registering current broker to name server completed"

	// rocketmqReadyTimeout bounds the wait for rocketmqRegisteredLine; it
	// takes some twenty seconds on a warm machine.
	rocketmqReadyTimeout = 2 * time.Minute
)

// rocketmqBrokerConf is the broker's configuration file, filled with the
// address the broker announces itself at and the port it listens on, see
// SetupRocketMQ; the name server runs in the same container. Topics are
// created on first use, which a client relies on when it sends to a topic no
// test created.
const rocketmqBrokerConf = `brokerClusterName=DefaultCluster
brokerName=broker-a
brokerId=0
namesrvAddr=127.0.0.1:9876
brokerIP1=%s
listenPort=%s
autoCreateTopicEnable=true
`

// SetupRocketMQ starts a RocketMQ name server and broker in a container of
// their own and points the framework at them, returning the function that
// terminates the container.
//
// Like kafka's, the container is dedicated even by default and left to
// testcontainers alone, the reaper included. A shared container would be
// possible, a namespace is a slot the client applies by itself, but a
// resident broker and name server idle at about a gigabyte of memory and a
// tenth of a core, which the twenty seconds they would save per start do not
// buy.
//
// A client asks the name server for a topic's route and dials the broker at
// the address the broker registered, which is the one its configuration
// names, so that address has to be reachable from the test process: the
// container publishes the broker's port under the same number on the host,
// chosen free on the host before the container starts, and the broker
// announces the docker host at that port. The client reads a name server
// address as an IP literal, which is why a local daemon's host is written as
// 127.0.0.1 rather than localhost.
func SetupRocketMQ() (func() error, error) {
	muteContainerLog()
	ctx := context.Background()

	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to docker")
	}
	defer provider.Close()
	host, err := provider.DaemonHost(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve the docker host")
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.Wrap(err, "failed to pick a port for the rocketmq broker")
	}
	_, hostPort, err := net.SplitHostPort(listener.Addr().String())
	if err = errors.CombineErrors(err, listener.Close()); err != nil {
		return nil, errors.Wrap(err, "failed to pick a port for the rocketmq broker")
	}
	brokerPort, err := network.ParsePort(hostPort + "/tcp")
	if err != nil {
		return nil, errors.Wrap(err, "failed to pick a port for the rocketmq broker")
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started:      true,
		Image:        rocketmqImage,
		ExposedPorts: []string{rocketmqNameServerPort, brokerPort.String()},
		// testcontainers publishes an exposed port under a random host
		// port unless the host config binds it already; the broker's is
		// bound to its own number, see above.
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			hostConfig.PortBindings = network.PortMap{brokerPort: {{HostPort: hostPort}}}
		},
		Env: map[string]string{"HEAP_OPTS": rocketmqHeapOptions},
		Cmd: []string{"sh", "-c", "./mqnamesrv & exec ./mqbroker -c " + rocketmqBrokerConfPath},
		Files: []testcontainers.ContainerFile{{
			Reader:            strings.NewReader(fmt.Sprintf(rocketmqBrokerConf, host, hostPort)),
			ContainerFilePath: rocketmqBrokerConfPath,
			FileMode:          0o644,
		}},
		WaitingFor: wait.ForExec([]string{"grep", "-q", rocketmqRegisteredLine, rocketmqBrokerLog}).
			WithStartupTimeout(rocketmqReadyTimeout),
	})
	if err != nil {
		err = errors.Wrap(err, "failed to start rocketmq container")
		if c != nil {
			err = errors.CombineErrors(err, c.Terminate(ctx))
		}
		return nil, err
	}
	terminate := func() error { return c.Terminate(ctx) }

	_, nameServerPort, err := endpoint(ctx, c, rocketmqNameServerPort)
	if err != nil {
		return nil, errors.CombineErrors(errors.Wrap(err, "failed to resolve the rocketmq name server"), terminate())
	}
	// NameServerAddrs is a slice, which ApplyConfigToEnv skips; the variable
	// carries the comma-separated form the config layer splits back apart.
	addr := fmt.Sprintf("%s:%d", host, nameServerPort)
	os.Setenv(config.ROCKETMQ_NAME_SERVER_ADDRS, addr)
	os.Setenv(config.ROCKETMQ_ENABLED, "true")
	reportServiceReady("rocketmq", addr)

	return terminate, nil
}
