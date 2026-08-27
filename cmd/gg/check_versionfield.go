package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// CheckVersionFieldDeclarations reports model.Version declarations that
// deviate from the required shape: a NAMED top-level field carrying
// gorm:"not null;default:1". An embedded Version is not recognized by the
// framework and the lock silently does not engage; a missing default:1
// backfills adopted rows to zero and locks them out of Update. "gg gen"
// heals the named-field cases automatically — this check is the read-only
// net for code that was committed without running it. Model subtrees owned
// by copyable framework modules are skipped, like every model check.
func CheckVersionFieldDeclarations(ignore gitignore.Matcher) []string {
	findings, err := collectVersionFieldFindings(ignore)
	if err != nil {
		return []string{err.Error()}
	}

	var violations []string
	for _, finding := range findings {
		relPath := relativePath(finding.Path)
		if finding.Embedded {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: struct '%s' embeds model.Version; optimistic locking requires a named field: Version model.Version `gorm:\"%s\"` (an embedded Version is not recognized and the lock silently does not engage)",
				relPath, finding.Line, finding.Struct, versionRequiredTag,
			))
			continue
		}
		violations = append(violations, fmt.Sprintf(
			"%s:%d: field '%s.%s' (model.Version) is missing gorm setting(s) %s; run \"gg gen\" to fill in gorm:\"%s\" — default:1 backfills existing rows to a live version when the column is added",
			relPath, finding.Line, finding.Struct, finding.Field, strings.Join(finding.Missing, ", "), versionRequiredTag,
		))
	}
	return violations
}

// collectVersionFieldFindings walks the model directory and gathers every
// deviating model.Version declaration, in walk order.
func collectVersionFieldFindings(ignore gitignore.Matcher) ([]versionFieldFinding, error) {
	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return nil, nil
	}

	owned, err := copyableModuleOwners()
	if err != nil {
		return nil, fmt.Errorf("listing copyable framework modules: %w", err)
	}

	var findings []versionFieldFinding
	walkErr := walkProjectDir(modelDir, ignore, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if moduleOwnedPath(owned, modelDir, path) {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") || isGeneratedFileName(path) {
			return nil
		}
		fileFindings, err := scanVersionFieldFile(path)
		if err != nil {
			return err
		}
		findings = append(findings, fileFindings...)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walking model directory: %w", walkErr)
	}
	return findings, nil
}
