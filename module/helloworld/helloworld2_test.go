package helloworld_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/module/helloworld"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helloworld2BatchRsp is the structured batch response with data, options and
// summary fields.
type helloworld2BatchRsp struct {
	Data    []*helloworld.Helloworld2 `json:"data"`
	Options map[string]any            `json:"options"`
	Summary struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	} `json:"summary"`
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
			before: "hello world 2 patch update before",
			after:  "hello world 2 patch update after",
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
				hw, err = client.Post[helloworld.Helloworld2](cli, helloworld2Path, res1)
				require.NoError(t, err)
				check1(t, tt, hw)

			case "delete":
				createHelloworld2TestRecord(t, cli, res1)
				hw, err = client.Delete[helloworld.Helloworld2](cli, helloworld2Path+"/"+id, nil)
				require.NoError(t, err)
				check1(t, tt, hw)

			case "update":
				createHelloworld2TestRecord(t, cli, res1)
				hw, err = client.Put[helloworld.Helloworld2](cli, helloworld2Path+"/"+id, res1)
				require.NoError(t, err)
				check1(t, tt, hw)

			case "patch":
				createHelloworld2TestRecord(t, cli, res1)
				hw, err = client.Patch[helloworld.Helloworld2](cli, helloworld2Path+"/"+id, res1)
				require.NoError(t, err)
				check1(t, tt, hw)

			case "list":
				createHelloworld2TestRecord(t, cli, res1)
				list, listErr := client.Get[client.ListResult[*helloworld.Helloworld2]](cli, helloworld2Path)
				require.NoError(t, listErr)

				item := findHelloworld2TestRecord(list.Items, id)
				require.NotNil(t, item)
				check1(t, tt, item)

			case "get":
				createHelloworld2TestRecord(t, cli, res1)
				hw, err = client.Get[helloworld.Helloworld2](cli, helloworld2Path+"/"+id)
				require.NoError(t, err)
				check1(t, tt, hw)

			case "create_many":
				batch, err = client.Post[helloworld2BatchRsp](cli, helloworld2Path+"/batch", client.BatchItems([]*helloworld.Helloworld2{res1, res2}))
				require.NoError(t, err)
				check2(t, tt, batch)

			case "delete_many":
				createHelloworld2TestRecord(t, cli, res1)
				createHelloworld2TestRecord(t, cli, res2)
				batch, err = client.Delete[helloworld2BatchRsp](cli, helloworld2Path+"/batch", client.BatchIDs([]string{id, id2}))
				require.NoError(t, err)
				check2(t, tt, batch)

			case "update_many":
				createHelloworld2TestRecord(t, cli, res1)
				createHelloworld2TestRecord(t, cli, res2)
				batch, err = client.Put[helloworld2BatchRsp](cli, helloworld2Path+"/batch", client.BatchItems([]*helloworld.Helloworld2{res1, res2}))
				require.NoError(t, err)
				check2(t, tt, batch)

			case "patch_many":
				createHelloworld2TestRecord(t, cli, res1)
				createHelloworld2TestRecord(t, cli, res2)
				batch, err = client.Patch[helloworld2BatchRsp](cli, helloworld2Path+"/batch", client.BatchItems([]*helloworld.Helloworld2{res1, res2}))
				require.NoError(t, err)
				check2(t, tt, batch)
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
	_, err := client.Post[struct{}](cli, helloworld2Path, record)
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

	for _, hw := range batch.Data {
		assert.Equal(t, tt.before, hw.Before)
		assert.Equal(t, tt.after, hw.After)
	}
}
