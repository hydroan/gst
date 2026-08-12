package testutil

import (
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// utf8BOM is the byte order mark some CSV producers prepend so spreadsheet
// applications detect the encoding.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// DownloadCSV downloads path as a CSV export through cli and parses the
// attachment into records, stripping a leading UTF-8 BOM if present. The
// helper sets the CSV format parameter itself; opts carry the remaining
// query parameters, such as filters.
func DownloadCSV(t *testing.T, cli *client.Client, path string, opts ...client.RequestOption) [][]string {
	t.Helper()

	format := []client.RequestOption{client.WithQuery(consts.QUERY_FORMAT, "csv")}
	attachment, err := cli.Download(path, append(format, opts...)...)
	require.NoError(t, err)

	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(attachment.Content, utf8BOM))).ReadAll()
	require.NoError(t, err)
	return records
}
