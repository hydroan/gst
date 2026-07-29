package module

import (
	"github.com/hydroan/gst/module/iam"
)

func init() {
	iam.Register()

	// Baseline accounts are application data: create them explicitly through
	// the standard database chain in a startup hook such as
	// router.OnRoutesReady, using serviceiamaccount.NewPasswordCredential for
	// password hashing.
}
