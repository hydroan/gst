package membership_test

import (
	"testing"

	"multitenant/internal/testsupport"
	"multitenant/model"

	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// TestList covers GET /api/memberships, served by Lister in list.go: the
// tenants the caller holds a role in, whichever tenant the session acts
// in, and that tenant as current, the default tenant for a login naming
// none.
func TestList(t *testing.T) {
	dave := iamtest.Login(t, iam.LoginReq{Username: "dave", Password: testsupport.Password, TenantID: "alpha"})
	memberships, err := dave.Client.Get[model.MembershipListRsp](t.Context(), "/api/memberships")
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "beta"}, memberships.Tenants)
	require.Equal(t, "alpha", memberships.Current)

	bob := iamtest.Login(t, iam.LoginReq{Username: "bob", Password: testsupport.Password})
	memberships, err = bob.Client.Get[model.MembershipListRsp](t.Context(), "/api/memberships")
	require.NoError(t, err)
	require.Equal(t, []string{"alpha"}, memberships.Tenants)
	require.Equal(t, "default", memberships.Current)
}
