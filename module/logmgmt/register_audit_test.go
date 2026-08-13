package logmgmt

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestRequireAuditEnabled pins the fail-fast contract: a registered logmgmt
// module without the audit pipeline must fail startup instead of serving a
// silently empty operation log.
func TestRequireAuditEnabled(t *testing.T) {
	original := config.App.Audit.Enabled
	t.Cleanup(func() { config.App.Audit.Enabled = original })

	config.App.Audit.Enabled = true
	require.NoError(t, requireAuditEnabled())

	config.App.Audit.Enabled = false
	require.ErrorContains(t, requireAuditEnabled(), config.AUDIT_ENABLED)
}
