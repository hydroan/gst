package logger

import (
	"sync"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestStreamNameIsTheFileNameWithoutItsExtension pins the name a stream goes
// by in stdout mode: the file name without its extension, and "global" for a
// stream that names no file.
func TestStreamNameIsTheFileNameWithoutItsExtension(t *testing.T) {
	require.Equal(t, "global", streamName(""))
	require.Equal(t, "global", streamName("/dev/stdout"))
	require.Equal(t, "global", streamName("/dev/stderr"))
	require.Equal(t, "controller", streamName("controller.log"))
	require.Equal(t, "controller", streamName("/var/log/app/controller.log"))
	require.Equal(t, "access", streamName(" access.log "))
}

// TestConstructorsBuildEachLoggerFromItsOwnFile pins that a constructor
// names its logger after the file it was given whatever another constructor
// running at the same time was given: the configuration a logger is built
// from is the constructor's own, not a state the constructors share.
func TestConstructorsBuildEachLoggerFromItsOwnFile(t *testing.T) {
	original := config.App
	config.App = new(config.Config)
	config.App.Logger.Output = config.LoggerOutputStdout
	config.App.Logger.Level = "info"
	config.App.Logger.Format = "json"
	t.Cleanup(func() { config.App = original })
	// The loggers share the stdout sink, which stays bound to the stdout of
	// this test; stopping it leaves the next test a sink of its own.
	t.Cleanup(stopBufferedLogWriters)

	const rounds = 200
	var wg sync.WaitGroup
	names := make([][3]string, rounds)
	for i := range rounds {
		wg.Add(3)
		go func() {
			defer wg.Done()
			names[i][0] = New("alpha.log").zlog.Name()
		}()
		go func() {
			defer wg.Done()
			names[i][1] = NewGin("beta.log").Name()
		}()
		go func() {
			defer wg.Done()
			names[i][2] = NewZap("gamma.log").Name()
		}()
	}
	wg.Wait()
	for i := range rounds {
		require.Equal(t, [3]string{"alpha", "beta", "gamma"}, names[i])
	}
}
