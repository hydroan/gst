package ggcheck

import (
	"fmt"
	"os"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// ModelFileNameHyphens keeps hyphens out of model file names.
var ModelFileNameHyphens = Check{
	Name: "Model file name hyphens",
	Rule: "model file names must not contain hyphens (use underscores instead)",
	run:  checkModelFileNameHyphens,
}

// checkModelFileNameHyphens checks that model file names separate words with
// underscores rather than hyphens.
func checkModelFileNameHyphens(ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	err := ignore.Walk(ggconst.DirModel, func(path string, info os.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") || isGeneratedFileName(path) {
			return nil
		}
		fileName := strings.TrimSuffix(info.Name(), ".go")
		if strings.Contains(fileName, "-") {
			violations = append(violations, fmt.Sprintf("Model file '%s' should not contain hyphens (suggested: %s.go)",
				path, strings.ReplaceAll(fileName, "-", "_")))
		}
		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	return violations
}
