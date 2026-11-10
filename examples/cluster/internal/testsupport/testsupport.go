// Package testsupport is what the test packages share: the login of the root
// account the iam module seeds, which the tests log in with to call the
// routes and rpcs behind the session check and the authorization.
package testsupport

import (
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/module/iam"
)

// RootPassword is the password every test package's TestMain hands the iam
// module, as IAM_ROOT_PASSWORD, to seed the root account with.
const RootPassword = "root-for-tests"

// Root is the login of the root account. The authorization mounted behind
// the session check refuses a subject holding no role, and root holds the
// system role that reaches every route, which is what lets the tests call
// the routes without appointing anyone.
var Root = iam.LoginReq{Username: rbac.UserRoot, Password: RootPassword}
