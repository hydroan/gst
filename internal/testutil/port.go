package testutil

import (
	"net"
	"os"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
)

// serverPort is the port the test server listens on. It is picked when the
// package loads, before any package-level URL is built, so that a test can
// declare its endpoints as package-level variables.
var serverPort = mustFreeLocalPort()

// listenOnFreePort configures the HTTP server to listen on the port URL
// resolves to.
func listenOnFreePort() {
	os.Setenv(config.SERVER_LISTEN, "127.0.0.1")
	os.Setenv(config.SERVER_PORT, strconv.Itoa(serverPort))
}

func mustFreeLocalPort() int {
	port, err := freeLocalPort()
	if err != nil {
		panic(err)
	}
	return port
}

// freeLocalPort asks the kernel for an unused port by binding to port zero and
// closing again. The port is then free for the test server to take.
func freeLocalPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		if err := l.Close(); err != nil {
			return 0, err
		}
		return 0, errors.Newf("unexpected listener address type %T", l.Addr())
	}
	port := addr.Port
	if err := l.Close(); err != nil {
		return 0, err
	}
	return port, nil
}
