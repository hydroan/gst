package ggcheck

import (
	"os"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinspect"
)

// QueryNames holds the models to query tags naming each URL query parameter
// once.
var QueryNames = Check{
	Name: "Query names",
	Rule: "a model's query tags must name each of its URL query parameters once: no two fields declare one name, a business field's name stays outside the \"_\" namespace of the framework's own parameters, and no tag holds \"[\", \"]\" or \".\", which the operator and the path syntax read",
	run:  checkQueryNames,
}

// checkQueryNames reports the models whose query tags name their URL query
// parameters ambiguously, which table preparation refuses at startup (see
// urlquery.ValidateQueryNames). The fields that take a name, columns among
// them, are gorm's and Go's to decide from the tags and the structs holding
// them, so the check asks the models themselves through the column inspection
// (see inspectedModelViolations).
func checkQueryNames(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}
	return inspectedModelViolations(ignore, "query names", func(model modelinspect.ModelColumns) string {
		return model.QueryNameViolation
	})
}
