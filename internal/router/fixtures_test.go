package router_test

import (
	"github.com/hydroan/gst/internal/testutil"
)

// baseURL addresses the test server. The port is picked per test binary, so it
// resolves once here instead of at every call site.
var baseURL = testutil.BaseURL()
