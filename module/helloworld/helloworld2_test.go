package helloworld_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/helloworld"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helloworld2BatchRsp is the structured batch response with its items.
type helloworld2BatchRsp struct {
	Items []*helloworld.Helloworld2 `json:"items"`
}

func TestHelloworld2Module(t *testing.T) {
	tests := []struct {
		name   string
		before string
		after  string
	}{
		{
			name:   "create",
			before: "hello world 2 create before",
			after:  "hello world 2 create after",
		},
		{
			name:   "delete",
			before: "",
			after:  "",
		},
		{
			name:   "update",
			before: "hello world 2 update before",
			after:  "hello world 2 update after",
		},
		{
			name:   "patch",
			before: "hello world 2 patch before",
			after:  "hello world 2 patch after",
		},

		{
			name:   "list",
			before: "hello world 2 list before",
			after:  "hello world 2 list after",
		},
		{
			name:   "get",
			before: "hello world 2 get before",
			after:  "hello world 2 get after",
		},
		{
			name:   "create_many",
			before: "hello world 2 batch create before",
			after:  "hello world 2 batch create after",
		},
		{
			name:   "delete_many",
			before: "",
			after:  "",
		},
		{
			name:   "update_many",
			before: "hello world 2 batch update before",
			after:  "hello world 2 batch update after",
		},
		{
			name:   "patch_many",
			before: "hello world 2 batch patch before",
			after:  "hello world 2 batch patch after",
		},
		{
			name:   "patch_many_missing_record",
			before: "",
			after:  "",
		},
		{
			name:   "patch_many_item_without_id",
			before: "",
			after:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := client.New(baseURL)
			require.NoError(t, err)

			suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
			id := "hw2_" + suffix
			id2 := "hw2b_" + suffix
			res1 := newHelloworld2TestRecord(id)
			res2 := newHelloworld2TestRecord(id2)

			var hw *helloworld.Helloworld2
			var batch *helloworld2BatchRsp

			switch tt.name {
			case "create":
				hw, err = cli.Post[helloworld.Helloworld2](t.Context(), helloworld2Path, res1)
				require.NoError(t, err)
				check1(t, tt, hw)

			case "delete":
				createHelloworld2TestRecord(t, cli, res1)
				hw, err = cli.Delete[helloworld.Helloworld2](t.Context(), helloworld2Path+"/"+id, nil)
				require.NoError(t, err)
				check1(t, tt, hw)

			case "update":
				createHelloworld2TestRecord(t, cli, res1)
				hw, err = cli.Put[helloworld.Helloworld2](t.Context(), helloworld2Path+"/"+id, res1)
				require.NoError(t, err)
				check1(t, tt, hw)

			case "patch":
				createHelloworld2TestRecord(t, cli, res1)
				hw, err = cli.Patch[helloworld.Helloworld2](t.Context(), helloworld2Path+"/"+id, res1)
				require.NoError(t, err)
				check1(t, tt, hw)

			case "list":
				createHelloworld2TestRecord(t, cli, res1)
				list, listErr := cli.Get[client.ListResult[*helloworld.Helloworld2]](t.Context(), helloworld2Path)
				require.NoError(t, listErr)

				item := findHelloworld2TestRecord(list.Items, id)
				require.NotNil(t, item)
				check1(t, tt, item)

			case "get":
				createHelloworld2TestRecord(t, cli, res1)
				hw, err = cli.Get[helloworld.Helloworld2](t.Context(), helloworld2Path+"/"+id)
				require.NoError(t, err)
				check1(t, tt, hw)

			case "create_many":
				batch, err = cli.Post[helloworld2BatchRsp](t.Context(), helloworld2Path+"/batch", client.BatchItems([]*helloworld.Helloworld2{res1, res2}))
				require.NoError(t, err)
				check2(t, tt, batch)

			case "delete_many":
				createHelloworld2TestRecord(t, cli, res1)
				createHelloworld2TestRecord(t, cli, res2)
				_, err = cli.Delete[helloworld2BatchRsp](t.Context(), helloworld2Path+"/batch", client.BatchIDs([]string{id, id2}))
				require.NoError(t, err)
				// A batch delete answers with no data: what shows it worked is
				// that neither record can be read any more.
				for _, deleted := range []string{id, id2} {
					_, getErr := cli.Get[helloworld.Helloworld2](t.Context(), helloworld2Path+"/"+deleted)
					testutil.RequireError(t, getErr, http.StatusNotFound)
				}

			case "update_many":
				createHelloworld2TestRecord(t, cli, res1)
				createHelloworld2TestRecord(t, cli, res2)
				batch, err = cli.Put[helloworld2BatchRsp](t.Context(), helloworld2Path+"/batch", client.BatchItems([]*helloworld.Helloworld2{res1, res2}))
				require.NoError(t, err)
				check2(t, tt, batch)

			case "patch_many":
				createHelloworld2TestRecord(t, cli, res1)
				createHelloworld2TestRecord(t, cli, res2)
				batch, err = cli.Patch[helloworld2BatchRsp](t.Context(), helloworld2Path+"/batch", client.BatchItems([]*helloworld.Helloworld2{res1, res2}))
				require.NoError(t, err)
				check2(t, tt, batch)

			case "patch_many_missing_record", "patch_many_item_without_id":
				createHelloworld2TestRecord(t, cli, res1)
				// The second item names a record that was never created,
				// which answers 404, or carries no id at all, a defective
				// request answered 400. Either way the batch patches all of
				// its items or none, so res1 keeps what its create stored.
				// The record is read from the database directly, as the get
				// hooks overwrite the fields they would show.
				other, status := res2, http.StatusNotFound
				if tt.name == "patch_many_item_without_id" {
					other, status = new(helloworld.Helloworld2), http.StatusBadRequest
				}
				_, err = cli.Patch[helloworld2BatchRsp](t.Context(), helloworld2Path+"/batch", client.BatchItems([]*helloworld.Helloworld2{res1, other}))
				testutil.RequireError(t, err, status)
				stored := new(helloworld.Helloworld2)
				require.NoError(t, database.Database[*helloworld.Helloworld2](context.Background()).Get(stored, id))
				assert.Equal(t, "hello world 2 create before", stored.Before)
			}
		})
	}
}

func newHelloworld2TestRecord(id string) *helloworld.Helloworld2 {
	record := new(helloworld.Helloworld2)
	record.SetID(id)
	return record
}

func createHelloworld2TestRecord(t *testing.T, cli *client.Client, record *helloworld.Helloworld2) {
	t.Helper()
	_, err := cli.Post[struct{}](t.Context(), helloworld2Path, record)
	require.NoError(t, err)
}

func findHelloworld2TestRecord(items []*helloworld.Helloworld2, id string) *helloworld.Helloworld2 {
	for _, item := range items {
		if item.GetID() == id {
			return item
		}
	}
	return nil
}

func check1(t *testing.T, tt struct {
	name   string
	before string

	after string
},
	hw *helloworld.Helloworld2,
) {
	t.Helper()
	pretty.Println(hw)

	assert.Equal(t, tt.before, hw.Before)
	assert.Equal(t, tt.after, hw.After)
}

func check2(t *testing.T, tt struct {
	name   string
	before string

	after string
},
	batch *helloworld2BatchRsp,
) {
	t.Helper()

	// Both submitted records come back; an empty list would pass the checks
	// below without checking anything.
	require.Len(t, batch.Items, 2)
	for _, hw := range batch.Items {
		assert.Equal(t, tt.before, hw.Before)
		assert.Equal(t, tt.after, hw.After)
	}
}
