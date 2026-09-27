// Package module assembles the application's modules: the built-in iam
// module, which serves signup, login and sessions. Its session check is
// mounted in middleware for the HTTP routes and in interceptor for the rpcs;
// the sessions live in redis, which every replica shares, so a session
// established on one replica is good on all of them.
package module

import "github.com/hydroan/gst/module/iam"

func init() {
	iam.Register()
}
