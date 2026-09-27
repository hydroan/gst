// Package module assembles the application's modules: the built-in iam
// module, which serves login, signup and sessions — its session check is
// mounted in middleware and interceptor — and helloworld, the example of a
// module of a project's own.
package module

import (
	"github.com/hydroan/gst/module/helloworld"
	"github.com/hydroan/gst/module/iam"
)

func init() {
	iam.Register()
	helloworld.Register()
}
