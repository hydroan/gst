package serviceregistry_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/stretchr/testify/require"
)

// TestBaseImportExportDefaultsRefuse pins the loud defaults: a route wired
// without its Import/Export service answers "not implemented" instead of
// silently importing nothing or exporting an empty file. The DSL already
// forces Service() on both actions; this is the runtime backstop for
// hand-wired registrations.
func TestBaseImportExportDefaultsRefuse(t *testing.T) {
	base := serviceregistry.Base[*modelregistry.Empty, any, any]{}

	_, err := base.Import(nil, nil)
	require.ErrorContains(t, err, "import service is not implemented")

	_, err = base.Export(nil)
	require.ErrorContains(t, err, "export service is not implemented")
}
