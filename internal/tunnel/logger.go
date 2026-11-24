package tunnel

import (
	"sync"

	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logger"
)

var (
	loggerOnce     sync.Once
	protocolLogger gst.Logger
	binaryLogger   gst.Logger
)

// protocolLog returns the tunnel's own protocol-event logger, built lazily on
// first use. The package is deliberately self-contained: the framework's
// logging setup knows nothing about the protocol and binary streams, so only a
// process that actually runs a tunnel session gets them.
func protocolLog() gst.Logger {
	ensureLoggers()
	return protocolLogger
}

// binaryLog returns the tunnel's own raw-frame logger; see protocolLog.
func binaryLog() gst.Logger {
	ensureLoggers()
	return binaryLogger
}

func ensureLoggers() {
	loggerOnce.Do(func() {
		protocolLogger = logger.New("protocol.log")
		binaryLogger = logger.New("binary.log")
	})
}
