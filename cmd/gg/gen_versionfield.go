package main

import (
	"fmt"
	"go/format"
	"os"
	"sort"
	"strings"

	"github.com/hydroan/gst/internal/clioutput"
	gormschema "gorm.io/gorm/schema"
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
				"%s:%d: struct '%s' embeds model.Version; optimistic locking requires a named field (Version model.Version `json:\"version,omitempty\" gorm:\"%s\"`) — gen cannot heal a field shape",
				relativePath(finding.Path), finding.Line, finding.Struct, versionRequiredTag)
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

// tagInsertion is one byte-offset insertion a finding's heal expands to.
type tagInsertion struct {
	offset int
	text   string
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

	var insertions []tagInsertion
	for _, finding := range findings {
		insertions = append(insertions, finding.insertions()...)
	}
	// Bottom-up keeps earlier offsets valid; the stable sort keeps one
	// finding's same-offset insertions in declaration order, which lands
	// them as ` json:"..." gorm:"..."` in the healed tag.
	sort.SliceStable(insertions, func(i, j int) bool { return insertions[i].offset > insertions[j].offset })
	for _, insertion := range insertions {
		if insertion.offset < 0 || insertion.offset > len(source) {
			return fmt.Errorf("%s: version tag rewrite offset out of range", relativePath(path))
		}
		source = append(source[:insertion.offset], append([]byte(insertion.text), source[insertion.offset:]...)...)
	}

	formatted, err := format.Source(source)
	if err != nil {
		return fmt.Errorf("%s: version tag rewrite produced unparsable code: %w", relativePath(path), err)
	}
	// The path comes from the model-directory walk and is fenced to it by
	// pathUnderRoot above; the taint analyzer cannot see through the fence.
	return os.WriteFile(safePath, formatted, stat.Mode().Perm()) //nolint:gosec
}

// insertions expands one healable finding into its byte insertions. The
// json name for a field without any json section follows gorm's naming
// strategy, so the wire name matches the column name a bare field gets.
func (finding versionFieldFinding) insertions() []tagInsertion {
	if !finding.hasTag {
		// No tag at all: both sections are missing by construction; add the
		// whole literal right after the field type.
		return []tagInsertion{{
			offset: finding.insertAfter,
			text:   " `json:\"" + versionJSONName(finding.Field) + ",omitempty\" gorm:\"" + versionRequiredTag + "\"`",
		}}
	}

	var insertions []tagInsertion
	if len(finding.Missing) > 0 {
		if finding.hasGormSection {
			// Append the missing settings inside the existing gorm value.
			insertions = append(insertions, tagInsertion{finding.gormValueEnd, ";" + strings.Join(finding.Missing, ";")})
		} else {
			// Add a gorm section before the literal's closing backquote.
			insertions = append(insertions, tagInsertion{finding.tagEnd - 1, ` gorm:"` + versionRequiredTag + `"`})
		}
	}
	if finding.JSONMissing {
		if finding.hasJSONSection {
			// Append omitempty inside the existing json value.
			insertions = append(insertions, tagInsertion{finding.jsonValueEnd, ",omitempty"})
		} else {
			insertions = append(insertions, tagInsertion{finding.tagEnd - 1, ` json:"` + versionJSONName(finding.Field) + `,omitempty"`})
		}
	}
	return insertions
}

// versionJSONName renders the wire name a healed json section uses.
func versionJSONName(fieldName string) string {
	return gormschema.NamingStrategy{}.ColumnName("", fieldName)
}
