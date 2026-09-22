package testutil_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/testutil"
)

// TestMain boots the framework against the default sqlite database and
// registers the sample model the database assertions run against.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Register: func() { modelregistry.Register[*SampleRecord]() },
	})
}
