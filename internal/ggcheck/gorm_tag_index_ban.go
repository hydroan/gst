package ggcheck

import (
	"fmt"
	"go/ast"

	"github.com/hydroan/gst/internal/gghelper"
	gormschema "gorm.io/gorm/schema"
	"gorm.io/gorm/utils"
)

// GormTagIndexBan keeps index settings out of gorm struct tags.
var GormTagIndexBan = Check{
	Name: "Gorm tag index ban",
	Rule: "gorm struct tags must not configure indexes (index, uniqueIndex, unique); models declare indexes through the Indexes() []model.Index method",
	run:  checkGormTagIndexBan,
}

// gormTag is the struct tag gorm reads the settings of a field from.
const gormTag = "gorm"

// checkGormTagIndexBan reports struct fields that configure indexes through
// gorm struct tags. Indexes are declared exclusively through the model's
// Indexes() []model.Index method, which validates the fields, generates the
// names, and feeds bootstrap and gg migrate from one definition; a tag index
// would bypass all of that. Only the primary key stays in tags, declared by
// the framework base structs themselves. The fields read are those of the
// project's own model files (see structFieldViolations).
func checkGormTagIndexBan(ignore gghelper.ProjectIgnore) []string {
	return structFieldViolations(ignore, func(structName string, field *ast.Field) string {
		key, ok := gormIndexTagKey(field)
		if !ok {
			return ""
		}
		return fmt.Sprintf(
			"field '%s.%s' configures an index through the gorm tag (%s); declare it through the model's Indexes() []model.Index method instead",
			structName, fieldDisplayName(field), key,
		)
	})
}

// gormIndexTagKey returns the index setting the field's gorm tag carries,
// named the way the rule names it. The tag is read by gorm's own parser: a
// key counts in any case, a semicolon escaped as \; keeps a word inside the
// setting it belongs to, and a value merely containing the word (a comment,
// a default) never matches. A setting counts when gorm acts on it: index and
// uniqueIndex carrying a value, the condition gorm's Schema.ParseIndexes
// builds an index on, and unique holding anything but false, gorm's
// Field.Unique; gorm exports the parser but not these conditions. unique is
// banned alongside index and uniqueIndex because it creates a unique index
// too and would reopen the side door the ban closes.
func gormIndexTagKey(field *ast.Field) (string, bool) {
	value, ok := structTagValue(field, gormTag)
	if !ok {
		return "", false
	}
	settings := gormschema.ParseTagSetting(value, ";")
	switch {
	case settings["INDEX"] != "":
		return "index", true
	case settings["UNIQUEINDEX"] != "":
		return "uniqueIndex", true
	case utils.CheckTruth(settings["UNIQUE"]):
		return "unique", true
	}
	return "", false
}
