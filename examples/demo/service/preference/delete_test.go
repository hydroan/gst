package preference_test

import (
	"net/http"
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestDelete covers DELETE /api/preferences/:id, served by the framework
// with the hook of Deleter in delete.go: the caller's preference goes, and
// someone else's is not there to delete.
func TestDelete(t *testing.T) {
	account := testsupport.Login(t)
	preference, err := account.Client.Post[model.PreferenceSetRsp](t.Context(), "/api/preferences/set", &model.PreferenceSetReq{Key: "theme", Value: "dark"})
	require.NoError(t, err)

	_, err = testsupport.Login(t).Client.Delete[struct{}](t.Context(), "/api/preferences/"+preference.ID, nil)
	testutil.RequireError(t, err, http.StatusNotFound)
	list, err := account.Client.Get[client.ListResult[*model.Preference]](t.Context(), "/api/preferences")
	require.NoError(t, err)
	require.Equal(t, 1, list.Total, "a delete refused leaves the preference")

	_, err = account.Client.Delete[struct{}](t.Context(), "/api/preferences/"+preference.ID, nil)
	require.NoError(t, err)
	list, err = account.Client.Get[client.ListResult[*model.Preference]](t.Context(), "/api/preferences")
	require.NoError(t, err)
	require.Equal(t, 0, list.Total)
}
