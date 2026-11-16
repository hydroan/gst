package product_test

import (
	"testing"

	"stress/internal/testsupport"
	"stress/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// TestExportRendersTheMatchingProductsAsCSV covers GET /api/products/export,
// served by Exporter in export.go: the products the query matches, one CSV
// row each under the header, nothing else.
func TestExportRendersTheMatchingProductsAsCSV(t *testing.T) {
	account := iamtest.Login(t, testsupport.Root)
	for _, product := range []*model.Product{
		{Code: "e-1", Title: "one", Category: "export", Status: 1, Score: 10, Body: "first body"},
		{Code: "e-2", Title: "two", Category: "export", Status: 2, Score: 20, Body: "second body"},
		{Code: "e-3", Title: "three", Category: "other", Status: 3, Score: 30, Body: "third body"},
	} {
		_, err := account.Client.Post[model.Product](t.Context(), "/api/products", product)
		require.NoError(t, err)
	}

	rows := testutil.DownloadCSV(t, account.Client, "/api/products/export", client.WithQuery("category", "export"), client.WithQuery("_sort_by", "code"))
	require.Len(t, rows, 3)
	require.Equal(t, []string{"id", "code", "title", "category", "status", "score", "body"}, rows[0])
	require.Equal(t, []string{"e-1", "one", "export", "1", "10", "first body"}, rows[1][1:])
	require.Equal(t, []string{"e-2", "two", "export", "2", "20", "second body"}, rows[2][1:])
}
