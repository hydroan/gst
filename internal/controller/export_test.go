package controller

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
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

// exportVirtualSample mirrors a virtual resource: query fields and the Empty
// marker, but no table behind it.
type exportVirtualSample struct {
	Name string `json:"name,omitempty" query:"name"`

	modelregistry.Query
	modelregistry.Empty
}

// exportVirtualSampleService is the exporter a virtual resource registers: it
// builds the export bytes itself and never receives controller-listed rows.
type exportVirtualSampleService struct {
	serviceregistry.Base[*exportVirtualSample, *exportVirtualSample, *exportVirtualSample]

	gotModels int
}

func (s *exportVirtualSampleService) Export(_ *types.ServiceContext, ms ...*exportVirtualSample) ([]byte, error) {
	s.gotModels = len(ms)
	return []byte("name\nsample\n"), nil
}

func TestExportFactoryVirtualModelSkipsListing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger.Controller = zap.New("")

	// No database is configured in this test on purpose: a virtual model has
	// no table, so the handler must never reach the controller-side listing.
	const route = "test/export_virtual_samples/export"
	svc := &exportVirtualSampleService{}
	serviceregistry.Register[*exportVirtualSample, *exportVirtualSample, *exportVirtualSample](consts.PHASE_EXPORT, route, svc)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/"+route+"?name=sample", nil)

	handler := ExportFactory[*exportVirtualSample, *exportVirtualSample, *exportVirtualSample](
		&types.ControllerConfig[*exportVirtualSample]{Route: route},
	)
	handler(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "name\nsample\n", rec.Body.String())
	require.Contains(t, rec.Header().Get("Content-Disposition"), csvTestName,
		"non-xlsx bytes must resolve to the csv attachment")
	require.Zero(t, svc.gotModels, "a virtual model export must not receive controller-listed rows")
}
