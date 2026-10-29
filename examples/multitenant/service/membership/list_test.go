package membership_test

import (
	"testing"

	"multitenant/internal/testsupport"
	"multitenant/model"

	"github.com/stretchr/testify/require"
)

// TestList covers GET /api/memberships, served by Lister in list.go: the
// tenants the caller holds a role in, whichever tenant the session acts
// in, and that tenant as current, the default tenant for a login naming
// none.
func TestList(t *testing.T) {
	dave := testsupport.Login(t, "dave", "alpha")
	memberships, err := dave.Client.Get[model.MembershipListRsp](t.Context(), "/api/memberships")
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "beta"}, memberships.Tenants)
	require.Equal(t, "alpha", memberships.Current)

	bob := testsupport.Login(t, "bob", "")
	memberships, err = bob.Client.Get[model.MembershipListRsp](t.Context(), "/api/memberships")
	require.NoError(t, err)
	require.Equal(t, []string{"alpha"}, memberships.Tenants)
	require.Equal(t, "default", memberships.Current)
}
