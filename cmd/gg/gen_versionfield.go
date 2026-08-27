package main

import (
	"fmt"
	"go/format"
	"os"
	"sort"
	"strings"

	"github.com/hydroan/gst/internal/clioutput"
)

// fillVersionFieldTags rewrites bare named model.Version fields under the
// model directory to carry the required gorm tag, printing each fix. It runs
// BEFORE the project checks so a bare declaration heals instead of failing
// the check gen itself runs first. The tag shape is a framework contract
// with no user freedom, which is what makes the rewrite legitimate: like the
// service skeleton correction, gen is normalizing a framework-owned
// declaration, not editing user logic.
//
// Embedded declarations cannot be healed by a tag — the required change is
// the field shape itself — so they abort generation with the same guidance
// the check gives.
func fillVersionFieldTags(quiet bool) error {
	findings, err := collectVersionFieldFindings(newProjectIgnoreMatcher())
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		return nil
	}

	byFile := make(map[string][]versionFieldFinding)
	for _, finding := range findings {
		if finding.Embedded {
			return fmt.Errorf(
				"%s:%d: struct '%s' embeds model.Version; optimistic locking requires a named field (Version model.Version `gorm:\"%s\"`) — gen cannot heal a field shape",
				relativePath(finding.Path), finding.Line, finding.Struct, versionRequiredTag)
		}
		byFile[finding.Path] = append(byFile[finding.Path], finding)
	}

	for path, fileFindings := range byFile {
		if err := rewriteVersionFieldTags(path, fileFindings); err != nil {
			return err
		}
		if !quiet {
			for _, finding := range fileFindings {
				clioutput.Success("FIX", "%s: filled gorm:\"%s\" on %s.%s",
					relativePath(path), versionRequiredTag, finding.Struct, finding.Field)
			}
		}
	}
	return nil
}

// rewriteVersionFieldTags applies the tag fixes of one file bottom-up, so
// earlier offsets stay valid, and writes the result back gofmt-formatted.
func rewriteVersionFieldTags(path string, findings []versionFieldFinding) error {
	safePath, err := pathUnderRoot(path, modelDir)
	if err != nil {
		return err
	}
	source, err := os.ReadFile(safePath)
	if err != nil {
		return err
	}
	stat, err := os.Stat(safePath)
	if err != nil {
		return err
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].insertAfter > findings[j].insertAfter })
	for _, finding := range findings {
		var offset int
		var insertion string
		switch {
		case finding.hasGormSection:
			// Append the missing settings inside the existing gorm value.
			offset = finding.gormValueEnd
			insertion = ";" + strings.Join(finding.Missing, ";")
		case finding.hasTag:
			// Add a gorm section to the existing tag literal, before its
			// closing backquote.
			offset = finding.tagEnd - 1
			insertion = ` gorm:"` + versionRequiredTag + `"`
		default:
			// No tag at all: add one right after the field type.
			offset = finding.insertAfter
			insertion = " `gorm:\"" + versionRequiredTag + "\"`"
		}
		if offset < 0 || offset > len(source) {
			return fmt.Errorf("%s: version tag rewrite offset out of range", relativePath(path))
		}
		source = append(source[:offset], append([]byte(insertion), source[offset:]...)...)
	}

	formatted, err := format.Source(source)
	if err != nil {
		return fmt.Errorf("%s: version tag rewrite produced unparsable code: %w", relativePath(path), err)
	}
	// The path comes from the model-directory walk and is fenced to it by
	// pathUnderRoot above; the taint analyzer cannot see through the fence.
	return os.WriteFile(safePath, formatted, stat.Mode().Perm()) //nolint:gosec
}
