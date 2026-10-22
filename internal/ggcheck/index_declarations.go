package ggcheck

import (
	"fmt"
	"os"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinspect"
)

// IndexDeclarations holds the models' Indexes() declarations to what the
// configured database accepts.
var IndexDeclarations = Check{
	Name: "Index declarations",
	Rule: "on a project configured for MySQL, the Indexes() declarations of every model must be ones MySQL accepts: the fields exist, no column repeats, no declaration duplicates a struct tag index, and every column can be indexed, which a string field without a size cannot, being a longtext column; give it a size (gorm:\"size:191\") or a bounded type",
	run:  checkIndexDeclarations,
}

// checkIndexDeclarations reports the index declarations the configured
// database refuses. Only MySQL refuses column types an index cannot cover, so
// the check runs when the project's configuration, read the way the service
// and gg migrate read it, names MySQL as the database, and otherwise has
// nothing to report. The column type a field gets is gorm's to decide from the
// field's tags and type, so the check asks the models themselves through the
// column inspection (see inspectedModelViolations) and reports what
// model.ValidateIndexes refused.
func checkIndexDeclarations(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}
	if err := config.Init(); err != nil {
		return []string{fmt.Sprintf("index declarations could not be checked: loading the configuration: %v", err)}
	}
	defer config.Clean()
	if config.App.Database.Type != config.DBMySQL {
		return nil
	}
	return inspectedModelViolations(ignore, "index declarations", func(model modelinspect.ModelColumns) string {
		return model.IndexViolation
	})
}
