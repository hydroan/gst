package bootstrap

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/provider"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// TestOptionalProvidersTableMatchesProviderDirectories keeps the warn table
// aligned with the provider/ directory: every optional provider package must
// have exactly one enable switch here, so adding a provider without wiring
// its configuration check fails in CI instead of drifting silently.
func TestOptionalProvidersTableMatchesProviderDirectories(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)

	entries, err := os.ReadDir(filepath.Join(filepath.Dir(file), "..", "provider"))
	require.NoError(t, err)

	dirs := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			dirs[entry.Name()] = true
		}
	}
	tabled := make(map[string]bool, len(optionalProviders))
	for name := range optionalProviders {
		tabled[name] = true
	}

	require.Equal(t, dirs, tabled)
}

func TestMissingProviders(t *testing.T) {
	original := config.App
	config.App = new(config.Config)
	t.Cleanup(func() { config.App = original })

	config.App.Elasticsearch.Enabled = true
	config.App.Kafka.Enabled = true

	require.Equal(t, []string{"elastic"}, missingProviders(map[string]bool{"kafka": true}))
	require.Empty(t, missingProviders(map[string]bool{"kafka": true, "elastic": true}))
}

// TestDrainProvidersWiresRegisteredProviders proves the drain hands Init to
// the initializer, adapts Close into a cleanup handler, assigns a dedicated
// logger through a declared handle before Init runs, and seals the registry.
// A provider without an Enabled function counts as enabled (the sample below
// declares none), while one whose Enabled reports false is left out of the
// lifecycle — no Init, no Close — though its logger handle is still bound.
// It mutates package-level bootstrap state, which is fine because bootstrap
// never runs inside this test binary.
func TestDrainProvidersWiresRegisteredProviders(t *testing.T) {
	original := config.App
	config.App = new(config.Config)
	config.App.Logger.Dir = t.TempDir()
	t.Cleanup(func() { config.App = original })

	initCalled := false
	closeCalled := false
	var handleLogger types.Logger
	provider.Register(provider.Provider{
		Name:   "test_drain_sample",
		Logger: &handleLogger,
		Init: func() error {
			// The dedicated logger must already be assigned when Init runs.
			initCalled = handleLogger != nil
			return nil
		},
		Close: func() error {
			closeCalled = true
			return errors.New("close sentinel")
		},
	})

	disabledInitCalled := false
	disabledCloseCalled := false
	var disabledLogger types.Logger
	provider.Register(provider.Provider{
		Name:    "test_drain_disabled",
		Enabled: func() bool { return false },
		Logger:  &disabledLogger,
		Init: func() error {
			disabledInitCalled = true
			return nil
		},
		Close: func() error {
			disabledCloseCalled = true
			return nil
		},
	})

	fnsBefore := len(ins.fns)
	handlersBefore := len(handlers)

	drainProviders()

	// Exactly one Init and one Close joined the lifecycle: the disabled
	// provider was skipped, so the single new entries belong to the enabled
	// sample.
	require.Len(t, ins.fns, fnsBefore+1)
	require.NoError(t, ins.fns[fnsBefore]())
	require.True(t, initCalled)
	require.False(t, disabledInitCalled)

	require.Len(t, handlers, handlersBefore+1)
	handlers[handlersBefore]()
	require.True(t, closeCalled)
	require.False(t, disabledCloseCalled)

	// The disabled provider keeps its dedicated logger binding: the handle
	// must not stay on the fallback just because the provider is off.
	require.NotNil(t, disabledLogger)

	// The dedicated logger's file name follows the registry name, and sink
	// construction precreates the file, so its existence proves the handle
	// points at the right sink.
	require.NotNil(t, handleLogger)
	require.FileExists(t, filepath.Join(config.App.Dir, "test_drain_sample.log"))

	// The drain seals the registry, so late registration must fail fast.
	require.Panics(t, func() {
		provider.Register(provider.Provider{Name: "test_drain_late", Init: func() error { return nil }})
	})
}
