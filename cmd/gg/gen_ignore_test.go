// Shared fixture helpers for the gst.yaml ignore tests. The route ignore
// tests live in gen_ignore_route_test.go and the model ignore tests in
// gen_ignore_model_test.go; this file keeps only the helpers both consume.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
)

// parseRules builds RouteRules from raw entries, failing the test on parse errors.
func parseRules(t *testing.T, raws ...string) []ggconfig.RouteRule {
	t.Helper()
	rules := make([]ggconfig.RouteRule, 0, len(raws))
	for _, raw := range raws {
		rule, err := ggconfig.ParseRouteRule(raw)
		if err != nil {
			t.Fatalf("ParseRouteRule(%q) error = %v", raw, err)
		}
		rules = append(rules, rule)
	}
	return rules
}

// findModelsFromSource writes source into a temporary project's model
// directory and scans it with codegen.FindModels, running the same
// endpoint/param setup genRunWithOptions performs before the ignore passes.
// This is the fallback construction path documented in the task brief: a
// directly built dsl.Design leaves undeclared action fields nil, which
// panics inside dsl.Design.Range, so tests must go through the DSL parser.
func findModelsFromSource(t *testing.T, pkgDir, filename, source string) []*gen.ModelInfo {
	t.Helper()
	projectDir := t.TempDir()
	fixtureModelDir := filepath.Join(projectDir, "model", pkgDir)
	if err := os.MkdirAll(fixtureModelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureModelDir, filename), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	// buildHierarchicalEndpoints derives directory-based endpoints by
	// trimming a literal "model/" prefix from ModelFilePath, so modelDir
	// must be passed as a relative path (matching how gg gen invokes it
	// from the project root) rather than the absolute t.TempDir() path.
	t.Chdir(projectDir)

	allModels, err := codegen.FindModels("tmpapp", "model", "service", nil)
	if err != nil {
		t.Fatal(err)
	}
	buildHierarchicalEndpoints(allModels)
	propagateParentParams(allModels)
	return allModels
}

// findDesign returns the Design for the named model, failing the test if
// it is not present among models.
func findDesign(t *testing.T, models []*gen.ModelInfo, modelName string) *dsl.Design {
	t.Helper()
	for _, m := range models {
		if m.ModelName == modelName {
			return m.Design
		}
	}
	t.Fatalf("model %q not found", modelName)
	return nil
}
