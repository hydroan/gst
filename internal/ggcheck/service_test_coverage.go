package ggcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// ServiceTestCoverage requires a test file for every service file generated
// for a DSL Service() action.
var ServiceTestCoverage = Check{
	Name: "Service test coverage",
	Rule: "service files generated for DSL Service() actions must have a matching test file (create.go pairs with create_test.go or create_internal_test.go)",
	run:  checkServiceTestCoverage,
}

// checkServiceTestCoverage checks that every service file generated for a DSL
// Service() action has a matching test file next to it: create.go pairs with
// create_test.go, or with create_internal_test.go for internal tests. The
// expected file list comes from the same DSL scan and route-ignore pipeline
// that drives gg gen, so route-ignored actions and service files gg gen has
// not generated yet are not reported. Service subtrees owned by copyable
// framework modules are skipped: copied module code is tested inside the
// framework repository and stays unmodified in projects.
func checkServiceTestCoverage(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}

	owned, err := copyableModuleOwners()
	if err != nil {
		return append(violations, fmt.Sprintf("listing copyable framework modules: %v", err))
	}
	cfg, err := ggconfig.Load(".")
	if err != nil {
		return append(violations, fmt.Sprintf("loading gst.yaml: %v", err))
	}
	allModels, err := codegen.FindModels(currentProjectModulePath(), ggconst.DirModel)
	if err != nil {
		return append(violations, fmt.Sprintf("scanning model designs: %v", err))
	}

	// Route-ignored actions are disabled here for the same reason gg gen
	// disables them: their service files stay on disk without a registered
	// route, so no test can exercise them.
	codegen.ResolveRoutes(allModels, cfg.Gen.Routes.Ignore)

	seen := make(map[string]bool)
	for _, m := range allModels {
		if isIgnoredProjectPath(ignore, m.ModelFilePath, false) {
			continue
		}
		m.Design.Range(func(_ string, act *dsl.Action) {
			if !act.Enabled || !act.Service {
				return
			}
			target := gen.ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
			if seen[target.FilePath] {
				return
			}
			seen[target.FilePath] = true

			if moduleOwnedPath(owned, ggconst.DirService, target.FilePath) || isIgnoredProjectPath(ignore, target.FilePath, false) {
				return
			}
			// A service file that does not exist yet is gg gen's business:
			// requiring its test here would block the gen run that scaffolds it.
			if !gghelper.FileExists(target.FilePath) {
				return
			}
			if serviceTestFileExists(target.FilePath) {
				return
			}

			stem := strings.TrimSuffix(filepath.Base(target.FilePath), ".go")
			violations = append(violations, fmt.Sprintf(
				"Service file '%s' has no matching test file (want %s_test.go or %s_internal_test.go)",
				target.FilePath, stem, stem,
			))
		})
	}

	return violations
}

// serviceTestFileExists reports whether a service file has a matching test
// file in its directory: <stem>_test.go or its internal form <stem>_internal_test.go.
func serviceTestFileExists(servicePath string) bool {
	stem := strings.TrimSuffix(servicePath, ".go")
	return gghelper.FileExists(stem+"_test.go") || gghelper.FileExists(stem+"_internal_test.go")
}
