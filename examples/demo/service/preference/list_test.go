package preference_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

// TestList covers GET /api/preferences, served by the framework with the
// hook of Lister in list.go: the caller's own preferences, and nobody
// else's.
func TestList(t *testing.T) {
	account := testsupport.Login(t)
	for _, key := range []string{"theme", "locale"} {
		_, err := account.Client.Post[model.PreferenceSetRsp](t.Context(), "/api/preferences/set", &model.PreferenceSetReq{Key: key, Value: "set"})
		require.NoError(t, err)
	}
	other := testsupport.Login(t)
	_, err := other.Client.Post[model.PreferenceSetRsp](t.Context(), "/api/preferences/set", &model.PreferenceSetReq{Key: "theme", Value: "other"})
	require.NoError(t, err)

	list, err := account.Client.Get[client.ListResult[*model.Preference]](t.Context(), "/api/preferences")
	require.NoError(t, err)
	require.Equal(t, 2, list.Total)
	for _, preference := range list.Items {
		require.Equal(t, account.UserID, preference.UserID)
	}

	list, err = other.Client.Get[client.ListResult[*model.Preference]](t.Context(), "/api/preferences")
	require.NoError(t, err)
	require.Equal(t, 1, list.Total)
	require.Equal(t, "other", list.Items[0].Value)
}
