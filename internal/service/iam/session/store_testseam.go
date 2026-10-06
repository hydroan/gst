// This file is a test seam: Store.Purge exists for the tests of this module
// and of the projects that copy it, which need a store with nothing in it.
// The framework's own paths never call it, since IAM only ever removes the
// sessions, user states and login counters of one user at a time, and the
// module manifest excludes this file from gg module copy so no project
// inherits it.

package serviceiamsession

import (
	"context"

	gstredis "github.com/hydroan/gst/redis"
)

// Purge drops every key IAM owns.
//
// It exists for tests that need a store with nothing in it. Every namespace is
// dropped, not just the session one: the user-state cache and the login failure
// counters are deliberately outside it, and a purge that left them would hand
// the next test a locked-out account.
func (store) Purge(ctx context.Context) error {
	if err := gstredis.RemovePrefix(ctx, sessionNamespace); err != nil {
		return err
	}
	if err := gstredis.RemovePrefix(ctx, userNamespace); err != nil {
		return err
	}
	return gstredis.RemovePrefix(ctx, loginNamespace)
}
