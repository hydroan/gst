package ggcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gertd/go-pluralize"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// ModelSingularNaming keeps model directory and file names singular.
var ModelSingularNaming = Check{
	Name: "Model singular naming",
	Rule: "model directories and files must be singular",
	run:  checkModelSingularNaming,
}

// checkModelSingularNaming checks that model directories and files use
// singular names.
func checkModelSingularNaming(ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	// Common plural file names that are allowed in Go projects. data and
	// stats name one body of content rather than many items; stats is the
	// conventional short form of statistics.
	allowedPluralFiles := map[string]bool{
		"types":       true,
		"errors":      true,
		"constants":   true,
		"consts":      true,
		"vars":        true,
		"handlers":    true,
		"models":      true,
		"examples":    true,
		"configs":     true,
		"options":     true,
		"helpers":     true,
		"utils":       true,
		"interfaces":  true,
		"services":    true,
		"clients":     true,
		"controllers": true,
		"apis":        true,
		"schemas":     true,
		"entities":    true,
		"records":     true,
		"data":        true,
		"stats":       true,
	}
	// Plural directory names that are allowed: types for a directory of
	// shared types, and data and stats, which name one body of content.
	allowedPluralDirs := map[string]bool{
		"types": true,
		"data":  true,
		"stats": true,
	}

	client := pluralize.NewClient()

	err := ignore.Walk(ggconst.DirModel, func(path string, info os.FileInfo) error {
		// Get relative path from model directory
		relPath, err := filepath.Rel(ggconst.DirModel, path)
		if err != nil {
			return err
		}

		// Skip the root model directory itself
		if relPath == "." {
			return nil
		}

		if info.IsDir() {
			// Check the directory name. Names of three letters or fewer are
			// not checked, and a name counts as plural only when it does not
			// also read as singular, so singular is checked first.
			dirName := info.Name()
			if len(dirName) > 3 && !allowedPluralDirs[dirName] && !client.IsSingular(dirName) && client.IsPlural(dirName) {
				violation := fmt.Sprintf("Model directory '%s' should be singular (suggested: %s)",
					path, client.Singular(dirName))
				violations = append(violations, violation)
			}
		} else if strings.HasSuffix(path, ".go") && !strings.Contains(path, "_test.go") {
			// Skip framework-generated files.
			if isGeneratedFileName(path) {
				return nil
			}

			// Check Go file name (without .go extension)
			fileName := strings.TrimSuffix(info.Name(), ".go")

			// As for directories, names of three letters or fewer are not
			// checked and singular is checked first; the allowed plural file
			// names are skipped.
			if len(fileName) > 3 && !allowedPluralFiles[fileName] && !client.IsSingular(fileName) && client.IsPlural(fileName) {
				violation := fmt.Sprintf("Model file '%s' should be singular (suggested: %s.go)",
					path, client.Singular(fileName))
				violations = append(violations, violation)
			}
		}

		return nil
	})
	if err != nil {
		violations = append(violations, fmt.Sprintf("walking model directory: %v", err))
	}

	return violations
}
