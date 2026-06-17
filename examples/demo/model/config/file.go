package config

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"

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
	NamespaceID string     `json:"namespace_id,omitempty" schema:"namespace_id"`
	Environment string     `json:"environment,omitempty" schema:"environment"`
	Name        string     `json:"name,omitempty" schema:"name"`
	Format      FileFormat `json:"format,omitempty" schema:"format"`
	Content     string     `json:"content,omitempty" schema:"content" gorm:"type:text"`
	Encrypted   bool       `json:"encrypted,omitempty" schema:"encrypted"`

	Size     int    `json:"size,omitempty" schema:"size"`
	Checksum string `json:"checksum,omitempty" schema:"checksum"`

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
			Service(true)
		})
		Update(func() {
			Service(true)
		})
		Patch(func() {})
		List(func() {
			Service(true)
		})
		Get(func() {})
	})

	Route("/config/namespaces/:namespace/files", func() {
		List(func() {})
	})
}

func (f *File) CreateBefore(ctx *types.ModelContext) error { return f.prepare(ctx) }
func (f *File) UpdateBefore(ctx *types.ModelContext) error { return f.prepare(ctx) }

func (f *File) prepare(_ *types.ModelContext) error {
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
