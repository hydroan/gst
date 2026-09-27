package document

import (
	"encoding/csv"
	"io"
	"net/http"

	"demo/model/archive"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Importer serves POST /api/archive/documents/import: the framework reads
// the uploaded file and hands it here.
type Importer struct {
	service.Base[*archive.Document, *archive.Document, *archive.Document]
}

// Import parses a CSV file of the columns box_id, name, format and content,
// under a header row, into documents, and returns them for the framework to
// create, in one transaction, the model hooks checking each. The service
// parses; the framework persists.
func (d *Importer) Import(_ *gst.ServiceContext, reader io.Reader) ([]*archive.Document, error) {
	rows, err := csv.NewReader(reader).ReadAll()
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusBadRequest, "the file is not valid csv", err)
	}
	if len(rows) < 2 {
		return nil, service.NewError(http.StatusBadRequest, "the file has no rows under its header")
	}
	documents := make([]*archive.Document, 0, len(rows)-1)
	for _, row := range rows[1:] {
		documents = append(documents, &archive.Document{
			BoxID:   row[0],
			Name:    row[1],
			Format:  archive.DocumentFormat(row[2]),
			Content: row[3],
		})
	}
	return documents, nil
}
