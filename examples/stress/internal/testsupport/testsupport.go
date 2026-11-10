// Package testsupport is what the test packages share: the login of the root
// account the iam module seeds.
package testsupport

import (
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/module/iam"
)

// RootPassword is the password TestMain hands the iam module, as
// IAM_ROOT_PASSWORD, to seed the root account with; the accounts the startup
// hook seeds get it too, as SEED_PASSWORD.
const RootPassword = "root-for-tests"

// Root is the login of the root account, which holds the system role and
// reaches every route.
var Root = iam.LoginReq{Username: rbac.UserRoot, Password: RootPassword}
