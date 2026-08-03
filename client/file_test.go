package client_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/stretchr/testify/require"
)

func TestDownloadReadsAttachment(t *testing.T) {
	var gotFormat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFormat = r.URL.Query().Get("_format")
		w.Header().Set("Content-Disposition", "attachment; filename=records.csv")
		w.Header().Set("Content-Type", "text/csv")
		fmt.Fprint(w, "name\nsample\n")
	}))
	t.Cleanup(srv.Close)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	attachment, err := cli.Download("/api/records/export", client.WithQuery("_format", "csv"))
	require.NoError(t, err)
	require.Equal(t, "csv", gotFormat)
	require.Equal(t, "records.csv", attachment.Name)
	require.Equal(t, "text/csv", attachment.ContentType)
	require.Equal(t, "name\nsample\n", string(attachment.Content))
}

func TestDownloadReturnsStructuredErrorOnRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"code":403,"msg":"permission denied"}`)
	}))
	t.Cleanup(srv.Close)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	_, err = cli.Download("/api/records/export")
	var respErr *client.Error
	require.True(t, errors.As(err, &respErr))
	require.Equal(t, http.StatusForbidden, respErr.StatusCode)
}

func TestUploadSendsMultipartFileAndFields(t *testing.T) {
	// The handler records errors instead of asserting: require inside a
	// handler goroutine cannot stop the test.
	var gotFile, gotField, gotName string
	var handlerErr error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handlerErr = r.ParseMultipartForm(1 << 20); handlerErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			handlerErr = err
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			handlerErr = err
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotFile = string(content)
		gotName = header.Filename
		gotField = r.FormValue("mode")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"msg":"success"}`)
	}))
	t.Cleanup(srv.Close)

	cli, err := client.New(srv.URL)
	require.NoError(t, err)

	resp, err := cli.Upload("/api/records/import", "records.csv",
		strings.NewReader("name\nsample\n"), map[string]string{"mode": "append"})
	require.NoError(t, err)
	require.NoError(t, handlerErr)
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "records.csv", gotName)
	require.Equal(t, "name\nsample\n", gotFile)
	require.Equal(t, "append", gotField)
}
