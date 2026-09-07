package dbruntime

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestInstallTracingRegistersPluginCallbacks pins what InstallTracing puts on
// a handle: the plugin's before and after callbacks around every statement
// verb, which is what makes each statement export a span.
func TestInstallTracingRegistersPluginCallbacks(t *testing.T) {
	db := newSQLiteDB(t)
	InstallTracing(db)

	cb := db.Callback()
	for _, p := range []struct {
		processor interface{ Get(string) func(*gorm.DB) }
		verb      string
	}{
		{cb.Create(), "create"},
		{cb.Query(), "select"},
		{cb.Delete(), "delete"},
		{cb.Update(), "update"},
		{cb.Row(), "row"},
		{cb.Raw(), "raw"},
	} {
		require.NotNil(t, p.processor.Get("otel:before:"+p.verb), "before callback for %s", p.verb)
		require.NotNil(t, p.processor.Get("otel:after:"+p.verb), "after callback for %s", p.verb)
	}
}
