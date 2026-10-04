package document

import (
	"bytes"
	"encoding/csv"
	"net/http"

	"demo/model/archive"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Exporter serves GET /api/archive/documents/export: the framework lists
// the documents the query parameters match and hands them here.
type Exporter struct {
	service.Base[*archive.Document, *archive.Document, *archive.Document]
}

// Export renders the documents as CSV, the columns Import reads; the
// framework answers with the bytes as a file attachment.
func (d *Exporter) Export(_ *gst.ServiceContext, documents ...*archive.Document) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	rows := [][]string{{"box_id", "name", "format", "content"}}
	for _, doc := range documents {
		rows = append(rows, []string{doc.BoxID, doc.Name, string(doc.Format), doc.Content})
	}
	if err := w.WriteAll(rows); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to render the csv", err)
	}
	return buf.Bytes(), nil
}
