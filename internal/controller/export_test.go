package controller

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	xlsxTestName = "exported.xlsx"
	xlsxTestMIME = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	csvTestName  = "exported.csv"
	csvTestMIME  = "text/csv; charset=utf-8"
)

func TestExportAttachment(t *testing.T) {
	tests := []struct {
		name         string
		format       string
		wantFilename string
		wantType     string
	}{
		{"xlsx", "xlsx", xlsxTestName, xlsxTestMIME},
		{"csv", "csv", csvTestName, csvTestMIME},
		{"empty defaults to xlsx", "", xlsxTestName, xlsxTestMIME},
		{"unknown defaults to xlsx", "pdf", xlsxTestName, xlsxTestMIME},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFilename, gotType := exportAttachment(tt.format)
			if gotFilename != tt.wantFilename {
				t.Errorf("filename = %q, want %q", gotFilename, tt.wantFilename)
			}
			if gotType != tt.wantType {
				t.Errorf("contentType = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

// minimalXLSX builds a tiny valid xlsx workbook so filetype detection reports xlsx.
func minimalXLSX(t *testing.T) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	entries := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/></Types>`},
		{"xl/workbook.xml", `<?xml version="1.0"?><workbook/>`},
	}
	for _, e := range entries {
		w, err := zw.Create(e.name)
		require.NoError(t, err)
		_, err = w.Write([]byte(e.body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestResolveExportFormat(t *testing.T) {
	xlsxBytes := minimalXLSX(t)
	csvBytes := append([]byte{0xEF, 0xBB, 0xBF}, []byte("账号,昵称\na1,昵称1\n")...)

	tests := []struct {
		name        string
		queryFormat string
		data        []byte
		want        string
	}{
		{"query xlsx wins over bytes", "xlsx", csvBytes, "xlsx"},
		{"query csv wins over bytes", "csv", xlsxBytes, "csv"},
		{"empty query sniffs xlsx bytes", "", xlsxBytes, "xlsx"},
		{"empty query sniffs csv bytes", "", csvBytes, "csv"},
		{"unknown query sniffs xlsx bytes", "pdf", xlsxBytes, "xlsx"},
		{"unknown query sniffs csv bytes", "pdf", csvBytes, "csv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveExportFormat(tt.queryFormat, tt.data); got != tt.want {
				t.Errorf("resolveExportFormat(%q, ...) = %q, want %q", tt.queryFormat, got, tt.want)
			}
		})
	}
}
