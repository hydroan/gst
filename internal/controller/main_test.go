package controller_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/testutil"
)

// TestMain brings the framework up on its default database with the fixture
// tables and services registered, so the handler tests read and write real
// rows through the factories they mount.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Register: func() {
			modelregistry.Register[*sampleRecord]()
			modelregistry.Register[*sampleCounter]()
			modelregistry.Register[*versionedSample]()
			registerFixtureServices()
		},
	})
}
