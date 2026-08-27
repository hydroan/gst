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
// json:",omitempty" serialization and gorm:"not null;default:1". An embedded
// Version is not recognized by the framework and the lock silently does not
// engage; a missing default:1 backfills adopted rows to zero and locks them
// out of Update; a json tag without omitempty serializes an unset version as
// an explicit zero the write paths reject, and json:"-" hides the version
// clients must hand back. "gg gen" heals the healable cases automatically —
// this check is the read-only net for code that was committed without
// running it. Model subtrees owned by copyable framework modules are
// skipped, like every model check.
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
				"%s:%d: struct '%s' embeds model.Version; optimistic locking requires a named field: Version model.Version `json:\"version,omitempty\" gorm:\"%s\"` (an embedded Version is not recognized and the lock silently does not engage)",
				relPath, finding.Line, finding.Struct, versionRequiredTag,
			))
			continue
		}
		if finding.JSONBlocked {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: field '%s.%s' (model.Version) carries json:\"-\"; the version must serialize so clients can hand it back",
				relPath, finding.Line, finding.Struct, finding.Field,
			))
			continue
		}
		missing := make([]string, 0, len(finding.Missing)+1)
		for _, setting := range finding.Missing {
			missing = append(missing, "gorm "+setting)
		}
		if finding.JSONMissing {
			missing = append(missing, "json omitempty")
		}
		violations = append(violations, fmt.Sprintf(
			"%s:%d: field '%s.%s' (model.Version) is missing %s; run \"gg gen\" to fill the tags in — default:1 backfills existing rows to a live version when the column is added, and omitempty keeps an unset version out of marshaled request bodies",
			relPath, finding.Line, finding.Struct, finding.Field, strings.Join(missing, ", "),
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
