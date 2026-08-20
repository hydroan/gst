package client_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

type sampleRecordRsp struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestVerbsDecodeEnvelopeData(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.Method + "-" + strings.TrimPrefix(r.URL.Path, "/api/records/")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":0,"data":{"name":"%s","count":2}}`, name)
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	got, err := cli.Get[sampleRecordRsp]("/api/records/one")
	require.NoError(t, err)
	require.Equal(t, "GET-one", got.Name)

	got, err = cli.Post[sampleRecordRsp]("/api/records/two", map[string]string{"name": "two"})
	require.NoError(t, err)
	require.Equal(t, "POST-two", got.Name)

	got, err = cli.Put[sampleRecordRsp]("/api/records/three", nil)
	require.NoError(t, err)
	require.Equal(t, "PUT-three", got.Name)

	got, err = cli.Patch[sampleRecordRsp]("/api/records/four", nil)
	require.NoError(t, err)
	require.Equal(t, "PATCH-four", got.Name)

	got, err = cli.Delete[sampleRecordRsp]("/api/records/five", nil)
	require.NoError(t, err)
	require.Equal(t, "DELETE-five", got.Name)
}

func TestVerbsDecodeEmptyDataToZeroValue(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"msg":"success"}`)
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	got, err := cli.Post[struct{}]("/api/records", nil)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestListResultDecodesItemsAndTotal(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"data":{"items":[{"name":"a","count":1},{"name":"b","count":2}],"total":7}}`)
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	list, err := cli.Get[client.ListResult[sampleRecordRsp]]("/api/records")
	require.NoError(t, err)
	require.Len(t, list.Items, 2)
	require.Equal(t, 7, list.Total)
	require.Equal(t, "a", list.Items[0].Name)
}

func TestBatchHelpersBuildStandardBodies(t *testing.T) {
	items, err := json.Marshal(client.BatchItems([]sampleRecordRsp{{Name: "a"}}))
	require.NoError(t, err)
	require.JSONEq(t, `{"items":[{"name":"a","count":0}]}`, string(items))

	ids, err := json.Marshal(client.BatchIDs([]string{"id1", "id2"}))
	require.NoError(t, err)
	require.JSONEq(t, `{"ids":["id1","id2"]}`, string(ids))
}

func TestGetSendsNoBody(t *testing.T) {
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":0,"data":{"count":%d}}`, len(body))
	}))
	srv.Start()

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	got, err := cli.Get[sampleRecordRsp]("/api/records")
	require.NoError(t, err)
	require.Zero(t, got.Count)
}
