package ggmodule

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
)

func TestListModulesReportsAddableCopyableAndPackageNames(t *testing.T) {
	projectDir := newModuleCommandProjectWithFramework(t)
	writeModuleManifestForTest(t, frameworkModuleDir(t, projectDir, "copytest"), `{}`)

	modules, err := ListModules()
	if err != nil {
		t.Fatalf("ListModules() error = %v", err)
	}

	byName := map[string]Module{}
	for _, module := range modules {
		byName[module.Name] = module
	}

	copytest := byName["copytest"]
	if copytest.Name != "copytest" || copytest.PackageName != "copytest" || !copytest.Addable || !copytest.Copyable {
		t.Fatalf("copytest module = %#v, want addable copyable package copytest", copytest)
	}

	aliased := byName["aliased"]
	if aliased.Name != "aliased" || aliased.PackageName != "aliasedmod" || !aliased.Addable || aliased.Copyable {
		t.Fatalf("aliased module = %#v, want addable non-copyable package aliasedmod", aliased)
	}

	configured := byName["configured"]
	if configured.Name != "configured" || configured.Addable || configured.Copyable {
		t.Fatalf("configured module = %#v, want non-addable non-copyable because Register requires arguments", configured)
	}
}

// TestCopiedModuleNamesAreTheCopyableModulesWithAModelSubtree pins which
// modules a project holds a copy of: the copyable ones whose model subtree
// the project has. A copyable module with no such directory was never
// copied, a file of its name is no subtree, and a model directory named
// after a module that cannot be copied is the project's own.
func TestCopiedModuleNamesAreTheCopyableModulesWithAModelSubtree(t *testing.T) {
	projectDir := newModuleCommandProjectWithFramework(t)
	for _, name := range []string{"copytest", "aliased"} {
		writeModuleManifestForTest(t, frameworkModuleDir(t, projectDir, name), `{}`)
	}

	copied, err := CopiedModuleNames()
	if err != nil || len(copied) != 0 {
		t.Fatalf("CopiedModuleNames() = %v, %v before anything is copied, want none", copied, err)
	}

	for _, name := range []string{"copytest", "configured"} {
		if err = os.MkdirAll(filepath.Join(projectDir, ggconst.DirModel, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(projectDir, ggconst.DirModel, "aliased"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	copied, err = CopiedModuleNames()
	if err != nil || !slices.Equal(copied, []string{"copytest"}) {
		t.Fatalf("CopiedModuleNames() = %v, %v, want copytest alone: aliased is a file and configured cannot be copied", copied, err)
	}
}
