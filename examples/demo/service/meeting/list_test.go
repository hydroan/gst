package meeting_test

import (
	"net/http"
	"testing"
	"time"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/datatypes"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestList covers GET /api/meetings, served by the framework with the hook
// of Lister in list.go, over the meetings the framework's own Create files:
// a meeting is answered as it was filed, its day, its time of day and its
// JSON columns each the value sent; tag keeps the meetings carrying it and
// upcoming=true the ones of the week ahead; the framework's own filters
// compare a day; and a meeting naming no day is refused.
func TestList(t *testing.T) {
	account := testsupport.Login(t)
	today := datatypes.Date(time.Now().UTC())
	startsAt := datatypes.NewTimeOfDay(17, 30, 0, 0)

	weekly, err := account.Client.Post[model.Meeting](t.Context(), "/api/meetings", &model.Meeting{
		Title:    "weekly sync",
		Day:      today.AddDays(2),
		StartsAt: &startsAt,
		Tags:     datatypes.JSONSlice[string]{"weekly", "planning"},
		Links:    datatypes.JSONMap{"agenda": "https://example.com/agenda"},
		Options:  datatypes.NewJSON(model.MeetingOptions{Online: true, Minutes: 30}),
	})
	require.NoError(t, err)
	retrospective, err := account.Client.Post[model.Meeting](t.Context(), "/api/meetings", &model.Meeting{
		Title: "retrospective",
		Day:   today.AddDays(30),
		Tags:  datatypes.JSONSlice[string]{"planning"},
	})
	require.NoError(t, err)

	t.Run("a meeting is answered as it was filed", func(t *testing.T) {
		got, err := account.Client.Get[model.Meeting](t.Context(), "/api/meetings/"+weekly.ID)
		require.NoError(t, err)
		require.Equal(t, today.AddDays(2).String(), got.Day.String())
		require.NotNil(t, got.StartsAt)
		require.Equal(t, "17:30:00", got.StartsAt.String())
		require.Equal(t, datatypes.JSONSlice[string]{"weekly", "planning"}, got.Tags)
		require.Equal(t, datatypes.JSONMap{"agenda": "https://example.com/agenda"}, got.Links)
		require.Equal(t, model.MeetingOptions{Online: true, Minutes: 30}, got.Options.Data())
	})

	t.Run("tag keeps the meetings carrying it", func(t *testing.T) {
		list, err := account.Client.Get[client.ListResult[*model.Meeting]](t.Context(), "/api/meetings", client.WithQuery("tag", "weekly"))
		require.NoError(t, err)
		require.Equal(t, 1, list.Total)
		require.Equal(t, weekly.ID, list.Items[0].ID)

		list, err = account.Client.Get[client.ListResult[*model.Meeting]](t.Context(), "/api/meetings", client.WithQuery("tag", "planning"))
		require.NoError(t, err)
		require.Equal(t, 2, list.Total)
	})

	t.Run("upcoming keeps the meetings of the week ahead", func(t *testing.T) {
		list, err := account.Client.Get[client.ListResult[*model.Meeting]](t.Context(), "/api/meetings", client.WithQuery("upcoming", true))
		require.NoError(t, err)
		require.Equal(t, 1, list.Total)
		require.Equal(t, weekly.ID, list.Items[0].ID)
	})

	t.Run("the framework's filters compare a day", func(t *testing.T) {
		list, err := account.Client.Get[client.ListResult[*model.Meeting]](t.Context(), "/api/meetings",
			client.WithQuery("day[gte]", today.AddDays(29).String(), "day[lte]", today.AddDays(31).String()))
		require.NoError(t, err)
		require.Equal(t, 1, list.Total)
		require.Equal(t, retrospective.ID, list.Items[0].ID)
	})

	t.Run("a meeting naming no day is refused", func(t *testing.T) {
		_, err := account.Client.Post[model.Meeting](t.Context(), "/api/meetings", &model.Meeting{Title: "no day"})
		testutil.RequireError(t, err, http.StatusBadRequest)
	})
}
