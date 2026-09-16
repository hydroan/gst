package archive

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"

	"github.com/cockroachdb/errors"
)

// DocumentFormat identifies the expected syntax of a document.
type DocumentFormat string

const (
	DocumentFormatText     DocumentFormat = "text"
	DocumentFormatJSON     DocumentFormat = "json"
	DocumentFormatYAML     DocumentFormat = "yaml"
	DocumentFormatMarkdown DocumentFormat = "markdown"
)

// Document demonstrates a database resource with custom routes and model hooks.
type Document struct {
	BoxID   string         `json:"box_id,omitempty" query:"box_id"`
	Label   string         `json:"label,omitempty" query:"label"`
	Name    string         `json:"name,omitempty" query:"name"`
	Format  DocumentFormat `json:"format,omitempty" query:"format"`
	Content string         `json:"content,omitempty" query:"content" gorm:"type:text"`
	Sealed  bool           `json:"sealed,omitempty" query:"sealed"`

	Size     int    `json:"size,omitempty" query:"size"`
	Checksum string `json:"checksum,omitempty" query:"checksum"`

	model.Base
}

func (Document) TableName() string { return "demo_archive_documents" }
func (Document) Purge() bool       { return true }

func (Document) Design() {
	Endpoint("documents")
	Param("document")
	Migrate()

	Route("/archive/documents", func() {
		Create(func() {
			Service()
		})
		Update(func() {
			Service()
		})
		Patch(func() {})
		List(func() {
			Service()
		})
		Get(func() {})
	})

	Route("/archive/boxes/:box/documents", func() {
		List(func() {})
	})
}

func (d *Document) CreateBefore(ctx context.Context) error { return d.prepare(ctx) }
func (d *Document) UpdateBefore(ctx context.Context) error { return d.prepare(ctx) }

func (d *Document) prepare(_ context.Context) error {
	if len(d.BoxID) == 0 {
		return errors.New("box id is required")
	}
	if len(d.Label) == 0 {
		return errors.New("label is required")
	}
	if len(d.Name) == 0 {
		return errors.New("document name is required")
	}
	if base := filepath.Base(d.Name); d.Name != base {
		return errors.Errorf("document name is invalid, expected %s", base)
	}

	d.Size = len(d.Content)
	d.Checksum = fmt.Sprintf("%x", sha256.Sum256([]byte(d.Content)))
	return nil
}
