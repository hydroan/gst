package main

import (
	"fmt"
	"go/format"
	"os"
	"sort"
	"strings"

	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/ggconst"
)

// fillVersionFieldTags rewrites named model.Version fields under the model
// directory to carry the required json and gorm tags, printing each fix. It
// runs BEFORE the project checks so a deviating declaration heals instead of
// failing the check gen itself runs first. The tag shape is a framework
// contract with no user freedom, which is what makes the rewrite legitimate:
// like the service skeleton correction, gen is normalizing a framework-owned
// declaration, not editing user logic.
//
// Two shapes cannot be healed and abort generation with the guidance the
// check gives: an embedded declaration (the required change is the field
// shape itself) and json:"-" (un-hiding a field its author silenced is a
// semantic decision no tool should make).
func fillVersionFieldTags(quiet bool) error {
	findings, err := ggcheck.VersionFieldFindings()
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		return nil
	}

	byFile := make(map[string][]ggcheck.VersionFieldFinding)
	for _, finding := range findings {
		if finding.Embedded {
			return fmt.Errorf(
				"%s:%d: struct '%s' embeds model.Version; optimistic locking requires a named field (Version model.Version `json:\"version,omitempty\" gorm:\"%s\"`) — gen cannot heal a field shape",
				relativePath(finding.Path), finding.Line, finding.Struct, ggcheck.VersionRequiredTag)
		}
		if finding.JSONBlocked {
			return fmt.Errorf(
				"%s:%d: field '%s.%s' (model.Version) carries json:\"-\"; the version must serialize so clients can hand it back — gen cannot un-hide a field its author silenced",
				relativePath(finding.Path), finding.Line, finding.Struct, finding.Field)
		}
		byFile[finding.Path] = append(byFile[finding.Path], finding)
	}

	for path, fileFindings := range byFile {
		if err := rewriteVersionFieldTags(path, fileFindings); err != nil {
			return err
		}
		if !quiet {
			for _, finding := range fileFindings {
				fixes := make([]string, 0, 2)
				if len(finding.Missing) > 0 {
					fixes = append(fixes, `gorm:"`+strings.Join(finding.Missing, ";")+`"`)
				}
				if finding.JSONMissing {
					fixes = append(fixes, `json:",omitempty"`)
				}
				clioutput.Success("FIX", "%s: filled %s on %s.%s",
					relativePath(path), strings.Join(fixes, " and "), finding.Struct, finding.Field)
			}
		}
	}
	return nil
}

// rewriteVersionFieldTags applies the tag fixes of one file bottom-up, so
// earlier offsets stay valid, and writes the result back gofmt-formatted.
func rewriteVersionFieldTags(path string, findings []ggcheck.VersionFieldFinding) error {
	safePath, err := pathUnderRoot(path, ggconst.DirModel)
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

	var insertions []ggcheck.TagInsertion
	for _, finding := range findings {
		insertions = append(insertions, finding.TagInsertions()...)
	}
	// Bottom-up keeps earlier offsets valid; the stable sort keeps one
	// finding's same-offset insertions in declaration order, which lands
	// them as ` json:"..." gorm:"..."` in the healed tag.
	sort.SliceStable(insertions, func(i, j int) bool { return insertions[i].Offset > insertions[j].Offset })
	for _, insertion := range insertions {
		if insertion.Offset < 0 || insertion.Offset > len(source) {
			return fmt.Errorf("%s: version tag rewrite offset out of range", relativePath(path))
		}
		source = append(source[:insertion.Offset], append([]byte(insertion.Text), source[insertion.Offset:]...)...)
	}

	formatted, err := format.Source(source)
	if err != nil {
		return fmt.Errorf("%s: version tag rewrite produced unparsable code: %w", relativePath(path), err)
	}
	// The path comes from the model-directory walk and is fenced to it by
	// pathUnderRoot above; the taint analyzer cannot see through the fence.
	return os.WriteFile(safePath, formatted, stat.Mode().Perm()) //nolint:gosec
}
