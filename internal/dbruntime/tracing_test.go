package dbruntime

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestInstallTracingFollowsTracingConfig pins the gate: with tracing
// configured off the handle stays bare, and with it on the plugin's before
// and after callbacks sit around every statement verb, which is what makes
// each statement export a span.
func TestInstallTracingFollowsTracingConfig(t *testing.T) {
	setTracing := func(t *testing.T, on bool) {
		t.Helper()
		original := config.App.OTEL.Enabled
		config.App.OTEL.Enabled = on
		t.Cleanup(func() { config.App.OTEL.Enabled = original })
	}

	t.Run("tracing_off_leaves_the_handle_bare", func(t *testing.T) {
		setTracing(t, false)
		db := newSQLiteDB(t)
		InstallTracing(db)
		for _, c := range pluginCallbacks(db) {
			require.Nil(t, c.fn, "%s must not be registered", c.name)
		}
	})

	t.Run("tracing_on_wraps_every_statement_verb", func(t *testing.T) {
		setTracing(t, true)
		db := newSQLiteDB(t)
		InstallTracing(db)
		for _, c := range pluginCallbacks(db) {
			require.NotNil(t, c.fn, "%s must be registered", c.name)
		}
	})
}

// pluginCallback is one of the plugin's callbacks looked up by name.
type pluginCallback struct {
	name string
	fn   func(*gorm.DB)
}

// pluginCallbacks looks up the plugin's before and after callbacks on every
// statement verb of db; a missing callback has a nil fn.
func pluginCallbacks(db *gorm.DB) []pluginCallback {
	cb := db.Callback()
	verbs := []struct {
		processor interface{ Get(string) func(*gorm.DB) }
		verb      string
	}{
		{cb.Create(), "create"},
		{cb.Query(), "select"},
		{cb.Delete(), "delete"},
		{cb.Update(), "update"},
		{cb.Row(), "row"},
		{cb.Raw(), "raw"},
	}
	callbacks := make([]pluginCallback, 0, 2*len(verbs))
	for _, v := range verbs {
		for _, side := range []string{"before", "after"} {
			name := "otel:" + side + ":" + v.verb
			callbacks = append(callbacks, pluginCallback{name: name, fn: v.processor.Get(name)})
		}
	}
	return callbacks
}
