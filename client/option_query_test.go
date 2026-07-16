package client_test

import (
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var addr = "http://localhost:8080"

func Test_OptionQuery(t *testing.T) {
	t.Run("WithQuery", func(t *testing.T) {
		cli, err := client.New(addr, client.WithQuery("name", "tom", "age", 20, "_sort_by", "created_at desc,name asc"))
		require.NoError(t, err)
		query, err := cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "name=tom&age=20&_sort_by=created_at+desc%2Cname+asc", query)

		cli, err = client.New(addr, client.WithQuery("name", "tom", "age", 20, "suname"))
		require.NoError(t, err)
		query, err = cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "name=tom&age=20", query)

		cli, err = client.New(addr, client.WithQuery("name", "tom", "age", 20, "suname"), client.WithQueryIndex("idx_composite_name_createdat"))
		require.NoError(t, err)
		query, err = cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "name=tom&age=20&_index=idx_composite_name_createdat", query)
	})

	t.Run("WithQueryPagination", func(t *testing.T) {
		cli, err := client.New(addr, client.WithQueryPagination(1, 10))
		require.NoError(t, err)
		query, err := cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "_page=1&_size=10", query)

		cli, err = client.New(addr, client.WithQueryPagination(1, -1))
		require.NoError(t, err)
		query, err = cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "_page=1&_size=-1", query)
	})

	t.Run("WithQueryExpand", func(t *testing.T) {
		cli, err := client.New(addr, client.WithQueryExpand("all", 3))
		require.NoError(t, err)
		query, err := cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "_depth=3&_expand=all", query)

		cli, err = client.New(addr, client.WithQueryExpand("children,parent", 3))
		require.NoError(t, err)
		query, err = cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "_depth=3&_expand=children%2Cparent", query)
	})

	t.Run("WithQuerySortBy", func(t *testing.T) {
		cli, err := client.New(addr, client.WithQuerySortBy("created_at desc,id asc"))
		require.NoError(t, err)
		query, err := cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "_sort_by=created_at+desc%2Cid+asc", query)
	})

	t.Run("WithQueryNoCache", func(t *testing.T) {
		cli, err := client.New(addr, client.WithQueryNoCache(true))
		require.NoError(t, err)
		query, err := cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "_no_cache=true", query)
	})

	t.Run("WithQueryOr", func(t *testing.T) {
		cli, err := client.New(addr, client.WithQueryOr(true))
		require.NoError(t, err)
		query, err := cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "_or=true", query)
	})

	t.Run("WithQueryIndex", func(t *testing.T) {
		cli, err := client.New(addr, client.WithQueryIndex("idx_composite_name_createdat"))
		require.NoError(t, err)
		query, err := cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "_index=idx_composite_name_createdat", query)
	})

	t.Run("WithQuerySelect", func(t *testing.T) {
		cli, err := client.New(addr, client.WithQuerySelect("name", "age", ""))
		require.NoError(t, err)
		query, err := cli.QueryString()
		require.NoError(t, err)
		assert.Equal(t, "_select=name%2Cage", query)
	})
}
