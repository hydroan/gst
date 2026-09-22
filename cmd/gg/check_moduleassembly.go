package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	gopath "path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/ggmodule"
	"github.com/hydroan/gst/internal/goast"
)

// CheckModuleAssembly reports assembly calls a copied framework module
// requires but the project never makes. gg module copy reproduces a module's
// routes, models and middleware; the rest of its Register body is the
// project's to write, and a missing call fails open — a login second-factor
// gate that is never installed lets every enrolled account in without a code.
// Modules declare the calls in module.json so the requirement is enforced here
// instead of living in a post-copy note nobody re-reads.
//
// The call must appear outside the module's own copied subtrees: code under
// model/<module> or service/<module> is only linked when generated
// registrations happen to import it, which is exactly the accident this check
// exists to prevent. Test files are skipped for the same reason — wiring that
// only runs under go test does not arm the binary.
func CheckModuleAssembly(ignore gitignore.Matcher) []string {
	// Cheapest first: a name whose model subtree is absent was never copied,
	// so a project that copied nothing reads no manifest and walks nothing.
	names, err := ggmodule.CopyableModuleNames()
	if err != nil {
		return []string{fmt.Sprintf("listing copyable framework modules: %v", err)}
	}
	copied := make([]string, 0, len(names))
	for _, name := range names {
		if info, statErr := os.Stat(filepath.Join(ggconst.DirModel, name)); statErr == nil && info.IsDir() {
			copied = append(copied, name)
		}
	}
	pending, err := ggmodule.RequiredAssembly(copied)
	if err != nil {
		return []string{fmt.Sprintf("reading framework module manifests: %v", err)}
	}
	if len(pending) == 0 {
		return nil
	}

	owned, err := copyableModuleOwners()
	if err != nil {
		return []string{fmt.Sprintf("listing copyable framework modules: %v", err)}
	}

	// Cheap first pass: a file that does not mention the function name by
	// bytes cannot call it, so only the rare hit is parsed.
	wanted := make(map[string]bool, len(pending))
	for _, call := range pending {
		wanted[call.Function] = true
	}
	satisfied := make(map[string]bool, len(pending))

	walkErr := walkProjectDir(".", ignore, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if moduleOwnedPath(owned, ggconst.DirModel, path) || moduleOwnedPath(owned, ggconst.DirService, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !mentionsAnyFunction(source, wanted) {
			return nil
		}
		collectAssemblyCalls(path, source, pending, satisfied)

		return nil
	})
	if walkErr != nil {
		return []string{fmt.Sprintf("walking project: %v", walkErr)}
	}

	violations := make([]string, 0)
	for _, call := range pending {
		if satisfied[assemblyKey(call)] {
			continue
		}
		violations = append(violations, fmt.Sprintf(
			"module %s is copied but the project never calls %s.%s: %s",
			call.Module, gopath.Base(call.Import), call.Function, call.Reason))
	}
	sort.Strings(violations)

	return violations
}

// assemblyKey identifies one required call.
func assemblyKey(call ggmodule.AssemblyCall) string { return call.Import + "." + call.Function }

// mentionsAnyFunction reports whether the source names any wanted function.
func mentionsAnyFunction(source []byte, wanted map[string]bool) bool {
	for name := range wanted {
		if bytes.Contains(source, []byte(name)) {
			return true
		}
	}

	return false
}

// collectAssemblyCalls marks every required call the file makes. A call counts
// only when it names the declared package the way this file imports it —
// under its package name, an alias or a dot import — so a same-named function
// from another package does not.
func collectAssemblyCalls(path string, source []byte, pending []ggmodule.AssemblyCall, satisfied map[string]bool) {
	file, err := parser.ParseFile(token.NewFileSet(), path, source, parser.SkipObjectResolution)
	if err != nil {
		// An unparsable file is reported by the compiler and by every other
		// check; treating it as "no call here" keeps this check quiet about it.
		return
	}

	names := make([]goast.PackageNames, len(pending))
	for i, required := range pending {
		names[i] = goast.ImportedNames(file, required.Import, gopath.Base(required.Import))
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		for i, required := range pending {
			if names[i].Refers(call.Fun, required.Function) {
				satisfied[assemblyKey(required)] = true
			}
		}

		return true
	})
}
