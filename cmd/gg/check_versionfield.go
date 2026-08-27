package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// CheckVersionFieldDeclarations reports model.Version declarations that
// deviate from the required shape. On database models that is a NAMED
// top-level field carrying json:",omitempty" serialization and
// gorm:"not null;default:1". An embedded Version is not recognized by the
// framework and the lock silently does not engage; a missing default:1
// backfills adopted rows to zero and locks them out of Update; a json tag
// without omitempty serializes an unset version as an explicit zero the
// write paths reject, and json:"-" hides the version clients must hand back.
// "gg gen" heals the healable cases automatically — this check is the
// read-only net for code that was committed without running it.
//
// On DSL action types (the Payload/Result types referenced by Design
// methods, plus the same-package types reachable from their fields) only the
// json half applies, pinned to the exact wire form json:"version,omitempty":
// hand-written DTOs are contracts gg gen never rewrites, and a missing or
// mismatched name stays green in Go-side tests — both ends marshal the same
// struct — while real clients sending "version" never bind it. Model
// subtrees owned by copyable framework modules are skipped, like every
// model check.
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

	actionFindings, err := collectActionTypeVersionFindings(ignore)
	if err != nil {
		return append(violations, err.Error())
	}
	for _, finding := range actionFindings {
		relPath := relativePath(finding.Path)
		if finding.Blocked {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: field '%s.%s' (model.Version) in a DSL action type carries json:\"-\"; the version must serialize so clients can hand it back",
				relPath, finding.Line, finding.Struct, finding.Field,
			))
			continue
		}
		got := ""
		if finding.HasJSON {
			got = fmt.Sprintf(" (got json:%q)", finding.Got)
		}
		violations = append(violations, fmt.Sprintf(
			"%s:%d: field '%s.%s' (model.Version) in a DSL action type must carry json:\"version,omitempty\"%s; the version handshake is wire-named \"version\", and a missing or mismatched name stays green in Go-side tests while real clients never bind it",
			relPath, finding.Line, finding.Struct, finding.Field, got,
		))
	}
	return violations
}

// collectActionTypeVersionFindings gathers the deviating model.Version
// fields of DSL action types, package by package: a Design method may
// reference a type declared in a sibling file, so files are grouped per
// directory the same way the json tag naming check groups them.
func collectActionTypeVersionFindings(ignore gitignore.Matcher) ([]actionTypeVersionFinding, error) {
	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return nil, nil
	}

	owned, err := copyableModuleOwners()
	if err != nil {
		return nil, fmt.Errorf("listing copyable framework modules: %w", err)
	}

	var packageDirs []string
	packageFiles := make(map[string][]string)
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
		dir := filepath.Dir(path)
		if _, seen := packageFiles[dir]; !seen {
			packageDirs = append(packageDirs, dir)
		}
		packageFiles[dir] = append(packageFiles[dir], path)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walking model directory: %w", walkErr)
	}

	var findings []actionTypeVersionFinding
	for _, dir := range packageDirs {
		findings = append(findings, scanPackageActionTypeVersionFields(packageFiles[dir])...)
	}
	return findings, nil
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
