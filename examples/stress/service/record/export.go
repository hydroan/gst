package record

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"strconv"

	"stress/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Exporter serves GET /api/records/export: the framework lists the records
// the query parameters match and hands them here, all at once.
type Exporter struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

// Export renders the records as CSV, one row per record with every column
// a client reads; the framework answers with the bytes as a file attachment.
func (r *Exporter) Export(_ *gst.ServiceContext, records ...*model.Record) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	rows := make([][]string, 0, len(records)+1)
	rows = append(rows, []string{"id", "code", "title", "category", "status", "score", "body"})
	for _, rec := range records {
		rows = append(rows, []string{rec.ID, rec.Code, rec.Title, rec.Category, strconv.Itoa(rec.Status), strconv.Itoa(rec.Score), rec.Body})
	}
	if err := w.WriteAll(rows); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to render the csv", err)
	}
	return buf.Bytes(), nil
}
