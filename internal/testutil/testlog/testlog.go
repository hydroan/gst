// Package testlog keeps the framework's logs of a test process out of the test
// output and out of the package source tree, where log files changing with
// every run would make go's test cache rerun every test that reads or lists
// the tree. testutil.Run points the logs away through ToTempDir, and so does
// every test package that bootstraps the framework or initializes the loggers
// on its own.
package testlog

import (
	"os"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	pkgzap "github.com/hydroan/gst/logger/zap"
)

// globalLogFile is the file the global stream writes to, named after what
// stdout mode calls the stream.
const globalLogFile = "global.log"

// ToTempDir points every log stream at a new scratch directory in file mode,
// and returns the directory with the function that removes it. It exports the
// settings through the environment, where config.Init reads them, and writes
// them to config.App.Logger as well, for a process that initializes the
// loggers without config.Init; either way it runs before the loggers come up.
//
// File mode alone does not keep the logs out of the test output: the global
// stream still writes to stdout when it names no file, and the console mirror
// copies it there when it names one. So the global stream gets a file of its
// own and the mirror is turned off.
//
// The files are not evidence a test can read back: the directory goes away at
// release, and every file sink buffers its entries (see the buffered writer in
// logger/zap), so a test process that ends within the flush interval leaves
// most of them empty. A test that needs to assert on log output instead swaps
// the package logger it cares about for a scratch file logger under its own
// t.TempDir and flushes that one before reading — see withCronjobLoggerConfig
// and readLogEntry in the cronjob tests.
//
// The release stops the log writers before it removes the directory. They hold
// entries back for up to a second, and a file writer opens its file on its
// first write, creating the directory when it is missing, so a write arriving
// after the removal would recreate the directory and leave it behind.
func ToTempDir() (dir string, release func() error, err error) {
	dir, err = os.MkdirTemp("", "gst_logs_")
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to create the test log directory")
	}

	os.Setenv(config.LOGGER_OUTPUT, string(config.LoggerOutputFile))
	os.Setenv(config.LOGGER_DIR, dir)
	os.Setenv(config.LOGGER_FILE, globalLogFile)
	os.Setenv(config.LOGGER_CONSOLE, "false")
	config.App.Logger.Output = config.LoggerOutputFile
	config.App.Logger.Dir = dir
	config.App.Logger.File = globalLogFile
	config.App.Logger.Console = false

	return dir, func() error {
		pkgzap.Clean()
		return os.RemoveAll(dir)
	}, nil
}
