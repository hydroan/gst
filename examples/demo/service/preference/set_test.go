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

// TestSet covers POST /api/preferences/set, served by Set in set.go: a key
// set twice is one row, its value the later one and its id the same, and a
// body without a key is refused before the service runs.
func TestSet(t *testing.T) {
	account := testsupport.Login(t)

	first, err := account.Client.Post[model.PreferenceSetRsp](t.Context(), "/api/preferences/set", &model.PreferenceSetReq{Key: "theme", Value: "dark"})
	require.NoError(t, err)
	require.NotEmpty(t, first.ID)
	require.Equal(t, account.UserID, first.UserID)
	require.Equal(t, "dark", first.Value)

	second, err := account.Client.Post[model.PreferenceSetRsp](t.Context(), "/api/preferences/set", &model.PreferenceSetReq{Key: "theme", Value: "light"})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "setting the key again replaces the value in place")
	require.Equal(t, "light", second.Value)

	list, err := account.Client.Get[client.ListResult[*model.Preference]](t.Context(), "/api/preferences")
	require.NoError(t, err)
	require.Equal(t, 1, list.Total)
	require.Equal(t, "light", list.Items[0].Value)

	_, err = account.Client.Post[model.PreferenceSetRsp](t.Context(), "/api/preferences/set", &model.PreferenceSetReq{Value: "no key"})
	testutil.RequireError(t, err, http.StatusBadRequest)
}
