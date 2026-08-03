package helloworld_test

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/module/helloworld"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			cli, err := client.New(addr2, client.WithToken(token))
			require.NoError(t, err)

			var resp *client.Resp

			suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
			id := "hw2_" + suffix
			id2 := "hw2b_" + suffix
			res1 := newHelloworld2TestRecord(id)
			res2 := newHelloworld2TestRecord(id2)

			switch tt.name {
			case "create":
				resp, err = cli.Create(res1)
				require.NoError(t, err)
				check1(t, tt, resp)

			case "delete":
				createHelloworld2TestRecord(t, cli, res1)
				resp, err = cli.Delete(id)
				require.NoError(t, err)
				check1(t, tt, resp)

			case "update":
				createHelloworld2TestRecord(t, cli, res1)
				resp, err = cli.Update(id, res1)
				require.NoError(t, err)
				check1(t, tt, resp)

			case "patch":
				createHelloworld2TestRecord(t, cli, res1)
				resp, err = cli.Patch(id, res1)
				require.NoError(t, err)
				check1(t, tt, resp)

			case "list":
				createHelloworld2TestRecord(t, cli, res1)
				items := make([]*helloworld.Helloworld2, 0)
				total := new(int)

				_, err = cli.List(&items, total)
				require.NoError(t, err)

				item := findHelloworld2TestRecord(items, id)
				require.NotNil(t, item)
				var data []byte
				data, err = json.Marshal(item)
				require.NoError(t, err)
				resp = &client.Resp{Data: data}

				check1(t, tt, resp)

			case "get":
				createHelloworld2TestRecord(t, cli, res1)
				hw := new(helloworld.Helloworld2)
				_, err = cli.Get(id, hw)
				require.NoError(t, err)

				// Convert hw to resp format for check1
				var data []byte
				data, err = json.Marshal(hw)
				require.NoError(t, err)
				resp = &client.Resp{Data: data}
				check1(t, tt, resp)

			case "create_many":
				resp, err = cli.CreateMany([]*helloworld.Helloworld2{res1, res2})
				require.NoError(t, err)
				check2(t, tt, resp)

			case "delete_many":
				createHelloworld2TestRecord(t, cli, res1)
				createHelloworld2TestRecord(t, cli, res2)
				resp, err = cli.DeleteMany([]string{id, id2})
				require.NoError(t, err)
				check2(t, tt, resp)

			case "update_many":
				createHelloworld2TestRecord(t, cli, res1)
				createHelloworld2TestRecord(t, cli, res2)
				resp, err = cli.UpdateMany([]*helloworld.Helloworld2{res1, res2})
				require.NoError(t, err)
				check2(t, tt, resp)

			case "patch_many":
				createHelloworld2TestRecord(t, cli, res1)
				createHelloworld2TestRecord(t, cli, res2)
				resp, err = cli.PatchMany([]*helloworld.Helloworld2{res1, res2})
				require.NoError(t, err)
				check2(t, tt, resp)
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
	_, err := cli.Create(record)
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
	resp *client.Resp,
) {
	t.Helper()
	hw := new(helloworld.Helloworld2)
	pretty.Println(string(resp.Data))
	if len(resp.Data) > 0 {
		require.NoError(t, json.Unmarshal(resp.Data, hw))
	}

	assert.Equal(t, tt.before, hw.Before)
	assert.Equal(t, tt.after, hw.After)
}

func check2(t *testing.T, tt struct {
	name   string
	before string

	after string
},
	resp *client.Resp,
) {
	t.Helper(
	// CreateMany returns a structured response with data, options, and summary fields
	)

	var batchResp struct {
		Data    []*helloworld.Helloworld2 `json:"data"`
		Options map[string]any            `json:"options"`
		Summary struct {
			Total     int `json:"total"`
			Succeeded int `json:"succeeded"`
			Failed    int `json:"failed"`
		} `json:"summary"`
	}
	if len(resp.Data) > 0 {
		require.NoError(t, json.Unmarshal(resp.Data, &batchResp))
	}

	for _, hw := range batchResp.Data {
		assert.Equal(t, tt.before, hw.Before)
		assert.Equal(t, tt.after, hw.After)
	}
}
