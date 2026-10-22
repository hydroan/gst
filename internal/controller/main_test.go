package controller_test

import (
	"os"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/testutil"
)

// TestMain brings the framework up on its default database with the fixture
// tables and services registered, so the handler tests read and write real
// rows through the handlers they mount.
func TestMain(m *testing.M) {
	// The transports are reached over the loopback, so naming it as a
	// trusted proxy lets a test forward a client address the way a proxy
	// in front would; the contract rows pin that both believe it alike.
	_ = os.Setenv("SERVER_TRUSTED_PROXIES", "127.0.0.1,::1")
	testutil.Run(m, testutil.Server{
		Register: func() {
			modelregistry.Register[*sampleRecord]()
			modelregistry.Register[*sampleCounter]()
			modelregistry.Register[*versionedSample]()
			modelregistry.Register[*validatedSample]()
			modelregistry.Register[*shapedSample]()
			modelregistry.Register[*nestedSample]()
			modelregistry.Register[*datedSample]()
			modelregistry.Register[*scopedSample]()
			registerFixtureServices()
		},
	})
}
