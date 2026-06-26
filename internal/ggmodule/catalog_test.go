package ggmodule

import "testing"

func TestListModulesFindsAddableModulesAndPackageNames(t *testing.T) {
	newModuleCommandProjectWithFramework(t)

	modules, err := ListModules()
	if err != nil {
		t.Fatalf("ListModules() error = %v", err)
	}

	byName := map[string]Module{}
	for _, module := range modules {
		byName[module.Name] = module
	}

	copytest := byName["copytest"]
	if copytest.Name != "copytest" || copytest.PackageName != "copytest" || !copytest.Addable {
		t.Fatalf("copytest module = %#v, want addable package copytest", copytest)
	}

	aliased := byName["aliased"]
	if aliased.Name != "aliased" || aliased.PackageName != "aliasedmod" || !aliased.Addable {
		t.Fatalf("aliased module = %#v, want addable package aliasedmod", aliased)
	}

	configured := byName["configured"]
	if configured.Name != "configured" || configured.Addable {
		t.Fatalf("configured module = %#v, want non-addable because Register requires arguments", configured)
	}
}
