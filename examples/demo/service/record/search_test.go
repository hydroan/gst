package record_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model"

	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

// TestSearch covers GET /api/records/search, served by Search in search.go:
// the query parameters of the framework's own lists, read by the service
// itself — a field of the model, in and notin filters, a prefix filter, an
// order and a page.
func TestSearch(t *testing.T) {
	account := testsupport.Login(t)
	createRecord(t, account, "draft one", model.RecordTypeText)
	createRecord(t, account, "draft two", model.RecordTypeImage)
	createRecord(t, account, "final", model.RecordTypeText)
	createRecord(t, testsupport.Login(t), "draft of someone else", model.RecordTypeText)

	t.Run("a field of the model", func(t *testing.T) {
		rsp, err := account.Client.Get[model.RecordSearchRsp](t.Context(), "/api/records/search", client.WithQuery("type", model.RecordTypeText))
		require.NoError(t, err)
		require.Equal(t, 2, rsp.Total)
	})

	t.Run("in and notin", func(t *testing.T) {
		rsp, err := account.Client.Get[model.RecordSearchRsp](t.Context(), "/api/records/search", client.WithQuery("type[in]", "text,image"))
		require.NoError(t, err)
		require.Equal(t, 3, rsp.Total)

		rsp, err = account.Client.Get[model.RecordSearchRsp](t.Context(), "/api/records/search", client.WithQuery("type[notin]", "image"))
		require.NoError(t, err)
		require.Equal(t, 2, rsp.Total)
	})

	t.Run("a prefix, an order and a page", func(t *testing.T) {
		rsp, err := account.Client.Get[model.RecordSearchRsp](t.Context(), "/api/records/search",
			client.WithQuery("title[startswith]", "draft"), client.WithSortBy("title desc"), client.WithPage(1, 1))
		require.NoError(t, err)
		require.Equal(t, 2, rsp.Total, "the total counts every match, not the page")
		require.Len(t, rsp.Items, 1)
		require.Equal(t, "draft two", rsp.Items[0].Title)
	})
}
