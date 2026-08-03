package helloworld_test

import (
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/helloworld"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var baseURL = testutil.URL("")

const (
	helloworldPath  = "/api/hello-world"
	helloworld2Path = "/api/hello-world2"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Register: helloworld.Register,
	})
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
			cli, err := client.New(baseURL)
			require.NoError(t, err)

			req := &helloworld.Req{
				Field1: "hello world",
				Field2: 0,
			}

			var rsp *helloworld.Rsp

			switch tt.name {
			case "create":
				rsp, err = client.Post[helloworld.Rsp](cli, helloworldPath, req)
			case "delete":
				rsp, err = client.Delete[helloworld.Rsp](cli, helloworldPath+"/123", nil)
			case "update":
				rsp, err = client.Put[helloworld.Rsp](cli, helloworldPath+"/123", req)
			case "patch":
				rsp, err = client.Patch[helloworld.Rsp](cli, helloworldPath+"/123", req)
			case "list":
				rsp, err = client.Get[helloworld.Rsp](cli, helloworldPath)
			case "get":
				rsp, err = client.Get[helloworld.Rsp](cli, helloworldPath+"/123")
			case "create_many":
				rsp, err = client.Post[helloworld.Rsp](cli, helloworldPath+"/batch", client.BatchItems([]helloworld.Req{*req}))
			case "delete_many":
				rsp, err = client.Delete[helloworld.Rsp](cli, helloworldPath+"/batch", client.BatchIDs([]string{}))
			case "update_many":
				rsp, err = client.Put[helloworld.Rsp](cli, helloworldPath+"/batch", client.BatchItems([]helloworld.Req{*req}))
			case "patch_many":
				rsp, err = client.Patch[helloworld.Rsp](cli, helloworldPath+"/batch", client.BatchItems([]helloworld.Req{*req}))
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, rsp.Field3)
		})
	}
}
