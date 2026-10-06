package testutil

import (
	"net"
	"os"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
)

// serverPort and grpcPort are the ports the test server's HTTP and gRPC
// listeners take. They are picked when the package loads, before any
// package-level URL is built, so that a test can declare its endpoints as
// package-level variables, and picked together, so that the kernel cannot
// hand the same port out twice.
var serverPort, grpcPort = mustFreeLocalPorts()

// loopbackHost is the address the test server listens on and its clients
// dial: only this process reaches it.
const loopbackHost = "127.0.0.1"

// listenOnFreePort configures the HTTP server to listen on the port URL
// resolves to, and the gRPC server on the one GRPCTarget resolves to.
func listenOnFreePort() {
	os.Setenv(config.SERVER_LISTEN, loopbackHost)
	os.Setenv(config.SERVER_PORT, strconv.Itoa(serverPort))
	os.Setenv(config.GRPC_LISTEN, loopbackHost)
	os.Setenv(config.GRPC_PORT, strconv.Itoa(grpcPort))
}

func mustFreeLocalPorts() (server, grpc int) {
	ports, err := freeLocalPorts(2)
	if err != nil {
		panic(err)
	}
	return ports[0], ports[1]
}

// freeLocalPorts asks the kernel for n unused ports by binding to port zero
// n times, holding every listener until all are bound so that no port comes
// back twice, and closing them again. The ports are then free for the test
// server to take.
func freeLocalPorts(n int) (ports []int, err error) {
	listeners := make([]net.Listener, 0, n)
	defer func() {
		for _, l := range listeners {
			err = errors.CombineErrors(err, l.Close())
		}
	}()
	for range n {
		l, listenErr := net.Listen("tcp", net.JoinHostPort(loopbackHost, "0"))
		if listenErr != nil {
			return nil, listenErr
		}
		listeners = append(listeners, l)
		addr, ok := l.Addr().(*net.TCPAddr)
		if !ok {
			return nil, errors.Newf("unexpected listener address type %T", l.Addr())
		}
		ports = append(ports, addr.Port)
	}
	return ports, nil
}
