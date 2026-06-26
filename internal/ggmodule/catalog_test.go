package ggmodule

import "testing"

func TestListModulesFindsAddableModulesAndPackageNames(t *testing.T) {
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
