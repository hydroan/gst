package config

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"

	"github.com/cockroachdb/errors"
)

// FileFormat identifies the expected syntax of a configuration file.
type FileFormat string

const (
	FileFormatText FileFormat = "text"
	FileFormatJSON FileFormat = "json"
	FileFormatYAML FileFormat = "yaml"
	FileFormatEnv  FileFormat = "env"
)

// File demonstrates a database resource with custom routes and model hooks.
type File struct {
	NamespaceID string     `json:"namespace_id,omitempty" query:"namespace_id"`
	Environment string     `json:"environment,omitempty" query:"environment"`
	Name        string     `json:"name,omitempty" query:"name"`
	Format      FileFormat `json:"format,omitempty" query:"format"`
	Content     string     `json:"content,omitempty" query:"content" gorm:"type:text"`
	Encrypted   bool       `json:"encrypted,omitempty" query:"encrypted"`

	Size     int    `json:"size,omitempty" query:"size"`
	Checksum string `json:"checksum,omitempty" query:"checksum"`

	model.Base
}

func (File) GetTableName() string { return "demo_config_files" }
func (File) Purge() bool          { return true }

func (File) Design() {
	Endpoint("files")
	Param("file")
	Migrate(true)

	Route("/config/files", func() {
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

	Route("/config/namespaces/:namespace/files", func() {
		List(func() {})
	})
}

func (f *File) CreateBefore(ctx context.Context) error { return f.prepare(ctx) }
func (f *File) UpdateBefore(ctx context.Context) error { return f.prepare(ctx) }

func (f *File) prepare(_ context.Context) error {
	if len(f.NamespaceID) == 0 {
		return errors.New("namespace id is required")
	}
	if len(f.Environment) == 0 {
		return errors.New("environment is required")
	}
	if len(f.Name) == 0 {
		return errors.New("file name is required")
	}
	if base := filepath.Base(f.Name); f.Name != base {
		return errors.Errorf("file name is invalid, expected %s", base)
	}

	f.Size = len(f.Content)
	f.Checksum = fmt.Sprintf("%x", sha256.Sum256([]byte(f.Content)))
	return nil
}
