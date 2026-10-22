package ggcheck

import (
	"os"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinspect"
)

// FrameworkColumns holds the models to the column names the framework fixes
// for its fields.
var FrameworkColumns = Check{
	Name: "Framework columns",
	Rule: "models must keep the framework's fields under the columns the framework fixes, id, created_by, updated_by, created_at, updated_at and deleted_at for the fields of model.Base and model.AutoBase and tenant_id for tenant.Scope: a model re-declaring one of those fields keeps its column, and embeds tenant.Scope without a prefix",
	run:  checkFrameworkColumns,
}

// checkFrameworkColumns reports the models mapping a framework field to a
// column of their own, which table preparation refuses at startup (see
// modelschema.ValidateFrameworkColumns), whatever database the project names.
// The column a field gets is gorm's to decide from the field's tags and the
// structs holding it, so the check asks the models themselves through the
// column inspection (see inspectedModelViolations).
func checkFrameworkColumns(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}
	return inspectedModelViolations(ignore, "framework columns", func(model modelinspect.ModelColumns) string {
		return model.FrameworkColumnViolation
	})
}
