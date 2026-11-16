package product

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"strconv"

	"stress/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Exporter serves GET /api/products/export: the framework lists the products
// the query parameters match and hands them here, all at once.
type Exporter struct {
	service.Base[*model.Product, *model.Product, *model.Product]
}

// Export renders the products as CSV, one row per product with every column
// a client reads; the framework answers with the bytes as a file attachment.
func (r *Exporter) Export(_ *gst.ServiceContext, products ...*model.Product) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	rows := make([][]string, 0, len(products)+1)
	rows = append(rows, []string{"id", "code", "title", "category", "status", "score", "body"})
	for _, product := range products {
		rows = append(rows, []string{product.ID, product.Code, product.Title, product.Category, strconv.Itoa(product.Status), strconv.Itoa(product.Score), product.Body})
	}
	if err := w.WriteAll(rows); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to render the csv", err)
	}
	return buf.Bytes(), nil
}
