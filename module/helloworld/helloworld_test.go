package helloworld_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/helloworld"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	token = "-"
	port  = testutil.SetupRandomServerPort()
	addr  = testutil.URL(port, "/api/hello-world")
	addr2 = testutil.URL(port, "/api/hello-world2")

	cleanDatabase func() error
)

func init() {
	var err error
	cleanDatabase, err = testutil.SetupMySQL()
	if err != nil {
		panic(err)
	}

	helloworld.Register()
	if err := bootstrap.Bootstrap(); err != nil {
		panic(err)
	}

	go func() {
		if err := bootstrap.Run(); err != nil {
			panic(err)
		}
	}()

	testutil.MustWaitForServer(port)
}

// TestMain releases the database every test in this package shares. Tearing it
// down from a single test would leave the remaining ones without a database.
func TestMain(m *testing.M) {
	code := m.Run()
	if cleanDatabase != nil {
		if err := cleanDatabase(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to release the test database: %v\n", err)
		}
	}
	os.Exit(code)
}

func TestHelloworldModule(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "create",
			want: "create hello world",
		},
		{
			name: "delete",
			want: "delete hello world",
		},
		{
			name: "update",
			want: "update hello world",
		},
		{
			name: "patch",
			want: "patch hello world",
		},
		{
			name: "list",
			want: "list hello world",
		},
		{
			name: "get",
			want: "get hello world",
		},
		{
			name: "create_many",
			want: "batch create hello world",
		},
		{
			name: "delete_many",
			want: "batch delete hello world",
		},
		{
			name: "update_many",
			want: "batch update hello world",
		},
		{
			name: "patch_many",
			want: "batch patch hello world",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := client.New(addr, client.WithToken(token))
			require.NoError(t, err)

			var resp *client.Resp

			switch tt.name {
			case "create":
				resp, err = cli.Create(&helloworld.Req{
					Field1: "hello world",
					Field2: 0,
				})
			case "delete":
				resp, err = cli.Delete("123")
			case "update":
				resp, err = cli.Update("123", &helloworld.Req{
					Field1: "hello world",
					Field2: 0,
				})
			case "patch":
				resp, err = cli.Patch("123", &helloworld.Req{
					Field1: "hello world",
					Field2: 0,
				})
			case "list":
				var req *http.Request
				var data []byte
				var httpResp *http.Response
				req, err = http.NewRequest(http.MethodGet, addr, nil)
				require.NoError(t, err)
				httpResp, err = http.DefaultClient.Do(req)
				require.NoError(t, err)
				defer httpResp.Body.Close()
				data, err = io.ReadAll(httpResp.Body)
				require.NoError(t, err)

				resp = &client.Resp{}
				require.NoError(t, json.Unmarshal(data, resp))

			case "get":
				resp, err = cli.Get("123", new(helloworld.Helloworld))
			case "create_many":
				resp, err = cli.CreateMany([]helloworld.Req{
					{
						Field1: "hello world",
						Field2: 0,
					},
				})
			case "delete_many":
				resp, err = cli.DeleteMany([]string{})
			case "update_many":
				resp, err = cli.UpdateMany([]helloworld.Req{
					{
						Field1: "hello world",
						Field2: 0,
					},
				})
			case "patch_many":
				resp, err = cli.PatchMany([]helloworld.Req{
					{
						Field1: "hello world",
						Field2: 0,
					},
				})
			}

			require.NoError(t, err)

			rsp := new(helloworld.Rsp)
			require.NoError(t, json.Unmarshal(resp.Data, rsp))

			assert.Equal(t, tt.want, rsp.Field3)
		})
	}
}
