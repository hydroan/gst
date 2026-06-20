package ggmodule

import "testing"

func TestListModulesFindsAddableModulesAndPackageNames(t *testing.T) {
	modules, err := ListModules()
	if err != nil {
		t.Fatalf("ListModules() error = %v", err)
	}

	byName := map[string]Module{}
	for _, module := range modules {
		byName[module.Name] = module
	}

	mfa := byName["mfa"]
	if mfa.Name != "mfa" || mfa.PackageName != "mfa" || !mfa.Addable {
		t.Fatalf("mfa module = %#v, want addable package mfa", mfa)
	}

	version := byName["version"]
	if version.Name != "version" || version.PackageName != "versionmod" || !version.Addable {
		t.Fatalf("version module = %#v, want addable package versionmod", version)
	}

	column := byName["column"]
	if column.Name != "column" || column.Addable {
		t.Fatalf("column module = %#v, want non-addable because Register requires arguments", column)
	}
}
