package ggcheck

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/ggconst"
)

// DSLDesignRules runs the Design() validation that gates gg gen over every
// model file.
var DSLDesignRules = Check{
	Name: "DSL design rules",
	Rule: "model files must pass the same validation rules that gate gg gen: the Design() DSL rules, and the base types model.Base, model.AutoBase and model.Empty embedded by value, never through a pointer",
	run:  checkDSLDesignRules,
}

// checkDSLDesignRules runs DSL Design() validation on every model file, so keyword
// placement and generation-semantic violations fail gg check with the same
// rules that block gg gen.
func checkDSLDesignRules(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	err := walkProjectDir(ggconst.DirModel, ignore, func(path string, info os.FileInfo) error {
		base := filepath.Base(path)
		if info.IsDir() {
			if path != ggconst.DirModel && (base == "vendor" || base == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(base, ".go") ||
			strings.HasSuffix(base, "_test.go") ||
			strings.HasPrefix(base, "_") {
			return nil
		}

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

	return violations
}
