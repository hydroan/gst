package ggmodule

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModuleCopyHelperDependencyFilesUsesTypes(t *testing.T) {
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

	got, err := moduleCopyHelperDependencyFiles(dir, []string{filepath.Join(dir, "action.go")})
	if err != nil {
		t.Fatalf("moduleCopyHelperDependencyFiles() error = %v", err)
	}

	if len(got) != 1 || filepath.Base(got[0]) != "helper.go" {
		t.Fatalf("moduleCopyHelperDependencyFiles() = %v, want only helper.go", got)
	}
}

func TestModuleCopyHelperDependencyFilesHandlesSymlinkedSourceDir(t *testing.T) {
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

	got, err := moduleCopyHelperDependencyFiles(linkDir, []string{filepath.Join(linkDir, "action.go")})
	if err != nil {
		t.Fatalf("moduleCopyHelperDependencyFiles() error = %v", err)
	}

	if len(got) != 1 || filepath.Base(got[0]) != "helper.go" {
		t.Fatalf("moduleCopyHelperDependencyFiles() = %v, want only helper.go", got)
	}
}

func TestModuleCopyHelperDependencyFilesFindsServiceHelpers(t *testing.T) {
	sourceServiceDir := writeCopyTestServiceDependencyFiles(t)
	actionFile := filepath.Join(sourceServiceDir, "create.go")

	got, err := moduleCopyHelperDependencyFiles(sourceServiceDir, []string{actionFile})
	if err != nil {
		t.Fatalf("moduleCopyHelperDependencyFiles() error = %v", err)
	}

	var found bool
	for _, file := range got {
		if filepath.Base(file) == "create_helper.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("moduleCopyHelperDependencyFiles() = %v, want create_helper.go", got)
	}
}

func TestModuleCopyHelperDependencyFilesFindsServiceHelpersThroughSymlink(t *testing.T) {
	sourceServiceDir := writeCopyTestServiceDependencyFiles(t)
	linkParent := t.TempDir()
	linkRoot := filepath.Join(linkParent, "servicecopytest")
	if symlinkErr := os.Symlink(sourceServiceDir, linkRoot); symlinkErr != nil {
		t.Skipf("symlink not available: %v", symlinkErr)
	}

	actionFile := filepath.Join(linkRoot, "create.go")
	got, err := moduleCopyHelperDependencyFiles(linkRoot, []string{actionFile})
	if err != nil {
		t.Fatalf("moduleCopyHelperDependencyFiles() error = %v", err)
	}

	var found bool
	for _, file := range got {
		if filepath.Base(file) == "create_helper.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("moduleCopyHelperDependencyFiles() = %v, want create_helper.go", got)
	}
}

func TestModuleCopyHelperDependencyFilesFindsServiceHelpersFromAllActions(t *testing.T) {
	sourceServiceDir := writeCopyTestServiceDependencyFiles(t)
	actionFiles := []string{
		filepath.Join(sourceServiceDir, "create.go"),
		filepath.Join(sourceServiceDir, "list.go"),
		filepath.Join(sourceServiceDir, "get.go"),
	}

	got, err := moduleCopyHelperDependencyFiles(sourceServiceDir, actionFiles)
	if err != nil {
		t.Fatalf("moduleCopyHelperDependencyFiles() error = %v", err)
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
			t.Fatalf("moduleCopyHelperDependencyFiles() = %v, want %s", got, file)
		}
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
