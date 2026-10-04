package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/hydroan/gst"
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// DocumentFormat is the syntax a document's content is in.
type DocumentFormat string

const (
	DocumentFormatText     DocumentFormat = "text"
	DocumentFormatMarkdown DocumentFormat = "markdown"
)

// Document is a table-backed resource served on routes it declares itself:
// /api/archive/documents and, for the documents of one box,
// /api/archive/boxes/:box_id/documents, a second List whose service hook
// keeps the list to the box of the route. Its checks and derived fields are
// model hooks, which run inside the framework's transaction whichever
// action writes the row. CSV import and export delegate to service code
// with fixed signatures, at /api/archive/documents/import and
// /api/archive/documents/export.
type Document struct {
	BoxID   string         `json:"box_id" query:"box_id"`
	Name    string         `json:"name" query:"name"`
	Format  DocumentFormat `json:"format" query:"format"`
	Content string         `json:"content,omitempty" gorm:"type:text"`
	// Size and Checksum derive from Content in the hooks.
	Size     int    `json:"size,omitempty"`
	Checksum string `json:"checksum,omitempty"`

	model.Base
}

func (Document) TableName() string { return "documents" }

func (Document) Design() {
	Migrate()
	Endpoint("documents")
	Param("document")

	Route("archive/documents", func() {
		Create(func() {})
		Delete(func() {})
		Update(func() {})
		Patch(func() {})
		List(func() {})
		Get(func() {})
		Import(func() {
			Service()
		})
		Export(func() {
			Service()
		})
	})
	Route("archive/boxes/:box_id/documents", func() {
		List(func() {
			Service("list_by_box")
		})
	})
}

func (d *Document) CreateBefore(context.Context) error { return d.derive() }
func (d *Document) UpdateBefore(context.Context) error { return d.derive() }

// derive checks the fields a document must have and fills the ones that
// follow from its content.
func (d *Document) derive() error {
	if d.BoxID == "" {
		return gst.NewError(http.StatusBadRequest, "box id is required")
	}
	if d.Name == "" {
		return gst.NewError(http.StatusBadRequest, "name is required")
	}
	if d.Format == "" {
		d.Format = DocumentFormatText
	}
	d.Size = len(d.Content)
	sum := sha256.Sum256([]byte(d.Content))
	d.Checksum = hex.EncodeToString(sum[:])
	return nil
}
