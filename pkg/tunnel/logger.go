package tunnel

import (
	"sync"

	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types"
)

var (
	loggerOnce     sync.Once
	protocolLogger types.Logger
	binaryLogger   types.Logger
)

// protocolLog returns the tunnel's own protocol-event logger, built lazily on
// first use. The package is deliberately self-contained: the framework's
// logging setup knows nothing about protocol.log and binary.log, so only a
// process that actually runs a tunnel session gets the files, and retiring
// this package will not touch the framework.
func protocolLog() types.Logger {
	ensureLoggers()
	return protocolLogger
}

// binaryLog returns the tunnel's own raw-frame logger; see protocolLog.
func binaryLog() types.Logger {
	ensureLoggers()
	return binaryLogger
}

func ensureLoggers() {
	loggerOnce.Do(func() {
		protocolLogger = pkgzap.New("protocol.log")
		binaryLogger = pkgzap.New("binary.log")
	})
}
