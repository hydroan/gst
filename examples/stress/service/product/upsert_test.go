package product_test

import (
	"testing"

	"stress/internal/testsupport"
	"stress/model"

	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// TestUpsertInsertsThenOverwritesTheProductOfTheCode covers POST
// /api/products/upsert, served by Upsert in upsert.go: the first call inserts
// the product under the id its code names, the second overwrites that row
// instead of adding one.
func TestUpsertInsertsThenOverwritesTheProductOfTheCode(t *testing.T) {
	cli := iamtest.Login(t, testsupport.Root).Client

	first, err := cli.Post[model.Product](t.Context(), "/api/products/upsert", &model.ProductUpsertReq{Code: "u-1", Title: "first", Category: "alpha", Score: 1})
	require.NoError(t, err)
	require.Equal(t, "sku-u-1", first.ID)

	second, err := cli.Post[model.Product](t.Context(), "/api/products/upsert", &model.ProductUpsertReq{Code: "u-1", Title: "second", Category: "alpha", Score: 2})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)

	stored, err := cli.Get[model.Product](t.Context(), "/api/products/sku-u-1")
	require.NoError(t, err)
	require.Equal(t, "second", stored.Title)
	require.Equal(t, 2, stored.Score)
}
