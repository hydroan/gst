package ggcheck

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"

	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
)

// DSLDesignRules runs the Design() validation that gates gg gen over every
// model file, and the route conflict check gg gen runs over them all.
var DSLDesignRules = Check{
	Name: "DSL design rules",
	Rule: "model files must pass the same validation rules that gate gg gen: the Design() DSL rules, the base types model.Base, model.AutoBase and model.Empty embedded by value, never through a pointer, and no two actions registering one path",
	run:  checkDSLDesignRules,
}

// checkDSLDesignRules runs DSL Design() validation on every model file, so keyword
// placement and generation-semantic violations fail gg check with the same
// rules that block gg gen, and then reports the conflicts among the routes
// of all the models the way gg gen refuses them (see
// modelinfo.RouteConflicts), the models read the way gg gen reads them,
// with the gst.yaml route ignores applied; a model tree that fails to load
// was reported file by file already.
func checkDSLDesignRules(ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	// The generator's own walk decides which model files take part, so a file
	// gg gen reads is a file this check validates, and nothing else is.
	err := modelinfo.WalkModelFiles(ggconst.DirModel, ignore, func(path string) error {
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			violations = append(violations, fmt.Sprintf("%s: %v", path, parseErr))
			return nil
		}
		for _, validateErr := range dsl.Validate(file, ggconst.DirModel, path) {
			violations = append(violations, validateErr.Error())
		}
		return nil
	})
	if err != nil {
		violations = append(violations, err.Error())
	}

	cfg, err := ggconfig.Load(".")
	if err != nil {
		return append(violations, fmt.Sprintf("loading gst.yaml: %v", err))
	}
	modulePath, err := gghelper.ModulePath()
	if err != nil {
		return append(violations, fmt.Sprintf("reading the module path: %v", err))
	}
	models, err := modelinfo.FindModels(modulePath, ggconst.DirModel, ignore)
	if err != nil {
		return violations
	}
	modelinfo.ResolveRoutes(models, cfg.Gen.Routes.Ignore)
	for _, conflict := range modelinfo.RouteConflicts(models) {
		violations = append(violations, conflict.Error())
	}
	return violations
}
