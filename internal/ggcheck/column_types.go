package ggcheck

import (
	"os"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinspect"
)

// ColumnTypes holds the columns of the models to the framework's types.
var ColumnTypes = Check{
	Name: "Column types",
	Rule: "a column of a kind the framework has a type for must be declared by that type of github.com/hydroan/gst/datatypes: a day, a time of day and a JSON document are sent, stored and answered the one way the framework tells by those types alone, Day datatypes.Date rather than Day time.Time under gorm:\"type:date\", and no column by the Date, Time, JSON, JSONMap, JSONType or JSONSlice of gorm.io/datatypes",
	run:  checkColumnTypes,
}

// checkColumnTypes reports the models declaring a column by a type the
// framework has its own for, which table preparation refuses at startup (see
// modelschema.ValidateColumnTypes), whatever database the project names.
// Which columns store a date is gorm's to decide from a field's type and its
// tags, so the check asks the models themselves through the column
// inspection (see inspectedModelViolations).
func checkColumnTypes(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}
	return inspectedModelViolations(ignore, "column types", func(model modelinspect.ModelColumns) string {
		return model.ColumnTypeViolation
	})
}
