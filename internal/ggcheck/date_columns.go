package ggcheck

import (
	"os"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinspect"
)

// DateColumns holds the date columns of the models to the framework's day.
var DateColumns = Check{
	Name: "Date columns",
	Rule: "every date column of a model must be declared gst.Date, or *gst.Date when it is optional: a day is sent, stored and answered as its text, 2026-01-02, by that type alone, Day gst.Date rather than Day datatypes.Date or Day time.Time under gorm:\"type:date\"",
	run:  checkDateColumns,
}

// checkDateColumns reports the models declaring a date column by a type
// other than gst.Date, which table preparation refuses at startup (see
// modelschema.ValidateDateColumns), whatever database the project names.
// Which columns store a date is gorm's to decide from a field's type and its
// tags, so the check asks the models themselves through the column
// inspection (see inspectedModelViolations).
func checkDateColumns(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}
	return inspectedModelViolations(ignore, "date columns", func(model modelinspect.ModelColumns) string {
		return model.DateColumnViolation
	})
}
