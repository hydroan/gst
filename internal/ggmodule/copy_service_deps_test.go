package ggmodule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleServiceHelperClosureUsesTypes(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module example.com/source\n\ngo 1.26\n")
	write("action.go", `package source

func Action() string {
	return helperValue
}
`)
	write("helper.go", `package source

const helperValue = "copied"
`)
	write("unused.go", `package source

const unusedValue = "kept out"
`)

	seeds, config := testServiceClosureConfig(t, dir, filepath.Join(dir, "action.go"))
	got, err := moduleServiceHelperClosure(seeds, config)
	if err != nil {
		t.Fatalf("moduleServiceHelperClosure() error = %v", err)
	}

	if len(got) != 1 || filepath.Base(got[0]) != "helper.go" {
		t.Fatalf("moduleServiceHelperClosure() = %v, want only helper.go", got)
	}
}

func TestModuleServiceHelperClosureHandlesSymlinkedSourceDir(t *testing.T) {
	realDir := t.TempDir()
	write := func(name string, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(realDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module example.com/source\n\ngo 1.26\n")
	write("action.go", `package source

func Action() string {
	return helperValue
}
`)
	write("helper.go", `package source

const helperValue = "copied"
`)

	linkParent := t.TempDir()
	linkDir := filepath.Join(linkParent, "source")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlink not available: %v", err)
	}

	seeds, config := testServiceClosureConfig(t, linkDir, filepath.Join(linkDir, "action.go"))
	got, err := moduleServiceHelperClosure(seeds, config)
	if err != nil {
		t.Fatalf("moduleServiceHelperClosure() error = %v", err)
	}

	if len(got) != 1 || filepath.Base(got[0]) != "helper.go" {
		t.Fatalf("moduleServiceHelperClosure() = %v, want only helper.go", got)
	}
}

func TestModuleServiceHelperClosureFindsServiceHelpers(t *testing.T) {
	sourceServiceDir := writeCopyTestServiceDependencyFiles(t)
	actionFile := filepath.Join(sourceServiceDir, "create.go")

	seeds, config := testServiceClosureConfig(t, sourceServiceDir, actionFile)
	got, err := moduleServiceHelperClosure(seeds, config)
	if err != nil {
		t.Fatalf("moduleServiceHelperClosure() error = %v", err)
	}

	var found bool
	for _, file := range got {
		if filepath.Base(file) == "create_helper.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("moduleServiceHelperClosure() = %v, want create_helper.go", got)
	}
}

func TestModuleServiceHelperClosureFindsServiceHelpersThroughSymlink(t *testing.T) {
	sourceServiceDir := writeCopyTestServiceDependencyFiles(t)
	linkParent := t.TempDir()
	linkRoot := filepath.Join(linkParent, "servicecopytest")
	if symlinkErr := os.Symlink(sourceServiceDir, linkRoot); symlinkErr != nil {
		t.Skipf("symlink not available: %v", symlinkErr)
	}

	actionFile := filepath.Join(linkRoot, "create.go")
	seeds, config := testServiceClosureConfig(t, linkRoot, actionFile)
	got, err := moduleServiceHelperClosure(seeds, config)
	if err != nil {
		t.Fatalf("moduleServiceHelperClosure() error = %v", err)
	}

	var found bool
	for _, file := range got {
		if filepath.Base(file) == "create_helper.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("moduleServiceHelperClosure() = %v, want create_helper.go", got)
	}
}

func TestModuleServiceHelperClosureFindsServiceHelpersFromAllActions(t *testing.T) {
	sourceServiceDir := writeCopyTestServiceDependencyFiles(t)
	actionFiles := []string{
		filepath.Join(sourceServiceDir, "create.go"),
		filepath.Join(sourceServiceDir, "list.go"),
		filepath.Join(sourceServiceDir, "get.go"),
	}

	seeds, config := testServiceClosureConfig(t, sourceServiceDir, actionFiles...)
	got, err := moduleServiceHelperClosure(seeds, config)
	if err != nil {
		t.Fatalf("moduleServiceHelperClosure() error = %v", err)
	}

	want := map[string]bool{
		"list_helper.go":   false,
		"create_helper.go": false,
		"shared_helper.go": false,
	}
	for _, file := range got {
		if _, ok := want[filepath.Base(file)]; ok {
			want[filepath.Base(file)] = true
		}
	}
	for file, found := range want {
		if !found {
			t.Fatalf("moduleServiceHelperClosure() = %v, want %s", got, file)
		}
	}
}

func TestModuleServiceHelperClosureCopiesBlankImportedPackageWholesale(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, content string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module example.com/source\n\ngo 1.26\n")
	write("service/service.go", `package service

type Base[M any, REQ any, RSP any] struct{}
`)
	write("action.go", `package source

import _ "example.com/source/sidefx"

func Action() string {
	return "action"
}
`)
	write("sidefx/init.go", `package sidefx

func init() {}
`)
	write("sidefx/extra.go", `package sidefx

func Extra() string {
	return "extra"
}
`)
	write("sidefx/svc.go", `package sidefx

import "example.com/source/service"

type SideService struct {
	service.Base[any, any, any]
}
`)

	seeds, config := testServiceClosureConfig(t, dir, filepath.Join(dir, "action.go"))
	got, err := moduleServiceHelperClosure(seeds, config)
	if err != nil {
		t.Fatalf("moduleServiceHelperClosure() error = %v", err)
	}

	want := map[string]bool{
		filepath.Join("sidefx", "init.go"):  false,
		filepath.Join("sidefx", "extra.go"): false,
	}
	for _, file := range got {
		for suffix := range want {
			if filepath.Base(filepath.Dir(file)) == filepath.Dir(suffix) && filepath.Base(file) == filepath.Base(suffix) {
				want[suffix] = true
			}
		}
		if filepath.Base(file) == "svc.go" {
			t.Fatalf("moduleServiceHelperClosure() = %v, must skip the service-struct file of a blank-imported package", got)
		}
	}
	for suffix, found := range want {
		if !found {
			t.Fatalf("moduleServiceHelperClosure() = %v, want blank-imported package file %s", got, suffix)
		}
	}
}

func TestModuleServiceHelperClosureRejectsReferencedServiceStructFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, content string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module example.com/source\n\ngo 1.26\n")
	write("service/service.go", `package service

type Base[M any, REQ any, RSP any] struct{}
`)
	write("action.go", `package source

func Action() string {
	return orphanShared()
}
`)
	// orphan_action.go declares a service struct but sits in no DSL action of
	// this plan, so the shared function inside it is unreachable for the copy.
	write("orphan_action.go", `package source

import "example.com/source/service"

type OrphanService struct {
	service.Base[any, any, any]
}

func orphanShared() string {
	return "shared"
}
`)

	seeds, config := testServiceClosureConfig(t, dir, filepath.Join(dir, "action.go"))
	_, err := moduleServiceHelperClosure(seeds, config)
	if err == nil || !strings.Contains(err.Error(), "declares a service struct") {
		t.Fatalf("moduleServiceHelperClosure() error = %v, want a service-struct reference error", err)
	}
}

func TestModuleServiceHelperClosureRejectsReferenceToExcludedFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module example.com/source\n\ngo 1.26\n")
	write("action.go", `package source

func Action() string {
	return helperValue
}
`)
	write("helper.go", `package source

const helperValue = "excluded"
`)

	excludedHelper, err := canonicalModuleCopyPath(filepath.Join(dir, "helper.go"))
	if err != nil {
		t.Fatal(err)
	}
	seeds, config := testServiceClosureConfig(t, dir, filepath.Join(dir, "action.go"))
	config.isExcluded = func(path string) bool { return path == excludedHelper }

	_, closureErr := moduleServiceHelperClosure(seeds, config)
	if closureErr == nil || !strings.Contains(closureErr.Error(), "excludeSourceFiles skips") {
		t.Fatalf("moduleServiceHelperClosure() error = %v, want an excluded-reference error", closureErr)
	}
}

// testServiceClosureConfig canonicalizes the given action files into closure
// seeds and builds the closure config a plan would derive for root.
func testServiceClosureConfig(t *testing.T, root string, actionPaths ...string) ([]string, moduleServiceClosureConfig) {
	t.Helper()

	actionFiles := make(map[string]bool, len(actionPaths))
	seeds := make([]string, 0, len(actionPaths))
	for _, path := range actionPaths {
		clean, err := canonicalModuleCopyPath(path)
		if err != nil {
			t.Fatal(err)
		}
		actionFiles[clean] = true
		seeds = append(seeds, clean)
	}
	return seeds, moduleServiceClosureConfig{
		serviceRoot:  root,
		importPrefix: "example.com/source",
		actionFiles:  actionFiles,
		isExcluded:   func(string) bool { return false },
		describe:     func(path string) string { return path },
	}
}

func writeCopyTestServiceDependencyFiles(t *testing.T) string {
	t.Helper()
	sourceServiceDir := t.TempDir()
	write := func(name string, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(sourceServiceDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/servicecopytest\n\ngo 1.26\n")
	write("create.go", `package servicecopytest

func Create() string {
	return createHelper() + sharedHelper()
}
`)
	write("list.go", `package servicecopytest

func List() string {
	return listHelper()
}
`)
	write("get.go", `package servicecopytest

func Get() string {
	return sharedHelper()
}
`)
	write("create_helper.go", `package servicecopytest

func createHelper() string {
	return "create"
}
`)
	write("list_helper.go", `package servicecopytest

func listHelper() string {
	return "list"
}
`)
	write("shared_helper.go", `package servicecopytest

func sharedHelper() string {
	return "shared"
}
`)
	return sourceServiceDir
}
