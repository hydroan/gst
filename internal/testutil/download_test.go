package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// serveCSV runs an export endpoint answering with body and hands back a
// client pointed at it, together with the query values the endpoint saw.
func serveCSV(t *testing.T, body []byte) (*client.Client, *map[string]string) {
	t.Helper()

	seen := make(map[string]string)
	ts := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, values := range r.URL.Query() {
			seen[key] = values[0]
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="sample.csv"`)
		if _, err := w.Write(body); err != nil {
			t.Errorf("failed to write the response body: %v", err)
		}
	}))
	ts.Start()

	cli, err := client.New(ts.URL)
	require.NoError(t, err)
	return cli, &seen
}

func TestDownloadCSVParsesRecordsAndSetsTheFormat(t *testing.T) {
	cli, seen := serveCSV(t, []byte("name,tag\nsample-a,one\nsample-b,two\n"))

	records := DownloadCSV(t, cli, "/api/samples/export", client.WithQuery("tag", "one"))

	require.Equal(t, [][]string{{"name", "tag"}, {"sample-a", "one"}, {"sample-b", "two"}}, records)
	require.Equal(t, "csv", (*seen)[consts.QUERY_FORMAT])
	require.Equal(t, "one", (*seen)["tag"])
}

func TestDownloadCSVStripsTheLeadingBOM(t *testing.T) {
	cli, _ := serveCSV(t, append(append([]byte{}, utf8BOM...), []byte("name\nsample-c\n")...))

	records := DownloadCSV(t, cli, "/api/samples/export")

	require.Equal(t, [][]string{{"name"}, {"sample-c"}}, records)
}
