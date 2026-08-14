package ggmodule

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func newModuleCommandProject(t *testing.T) string {
	t.Helper()
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module tmpapp\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	moduleDir := filepath.Join(projectDir, "module")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "module.go"), []byte(`package module

func init() {
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	return projectDir
}

func newModuleCommandProjectWithFramework(t *testing.T) string {
	t.Helper()
	projectDir := newModuleCommandProject(t)
	writeFakeFrameworkModule(t, projectDir, "copytest", "copytest", "")
	writeFakeFrameworkModule(t, projectDir, "aliased", "aliasedmod", "")
	writeFakeFrameworkModule(t, projectDir, "configured", "configured", "config string")
	t.Chdir(projectDir)
	return projectDir
}

func writeFakeFrameworkModule(t *testing.T, projectDir string, name string, packageName string, registerParam string) {
	t.Helper()
	moduleDir := frameworkModuleDir(t, projectDir, name)
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	signature := "func Register() {}"
	if registerParam != "" {
		signature = "func Register(" + registerParam + ") {}"
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "register.go"), []byte("package "+packageName+"\n\n"+signature+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	frameworkMod := filepath.Join(projectDir, "internal", "gst", "go.mod")
	if err := os.MkdirAll(filepath.Dir(frameworkMod), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(frameworkMod, []byte("module github.com/hydroan/gst\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func frameworkModuleDir(t *testing.T, projectDir string, name string) string {
	t.Helper()
	return filepath.Join(projectDir, "internal", "gst", "module", name)
}

func readProjectModuleFile(t *testing.T, projectDir string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectDir, "module", "module.go"))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func writeModuleManifestForTest(t *testing.T, moduleDir, content string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(moduleDir, moduleManifestFilename), []byte(content), 0o600)
	require.NoError(t, err)
}

func newModuleCopyPlanProject(t *testing.T) string {
	t.Helper()
	projectDir := t.TempDir()
	frameworkRoot := filepath.Join(projectDir, "internal", "gst")
	for _, dir := range []string{
		filepath.Join(frameworkRoot, "module", "copytest"),
		filepath.Join(frameworkRoot, "internal", "model", "copytest"),
		filepath.Join(frameworkRoot, "internal", "service", "copytest"),
		filepath.Join(frameworkRoot, "dsl"),
		filepath.Join(frameworkRoot, "model"),
		filepath.Join(frameworkRoot, "service"),
		filepath.Join(frameworkRoot, "types"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module tmpapp\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "go.mod"), []byte("module github.com/hydroan/gst\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "service", "base.go"), []byte(`package service

type Base[M any, REQ any, RSP any] struct{}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "types", "types.go"), []byte(`package types

type ServiceContext struct{}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "model", "empty.go"), []byte(`package model

type Empty struct{}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "dsl", "dsl.go"), []byte(`package dsl

func Route(string, func()) {}
func Create(func()) {}
func List(func()) {}
func Get(func()) {}
func Service(...bool) {}
func Filename(string) {}
func Result[T any]() {}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	return projectDir
}

func writeCopyTestModuleSource(t *testing.T, projectDir string, manifest []byte) {
	t.Helper()
	frameworkRoot := filepath.Join(projectDir, "internal", "gst")
	if manifest == nil {
		manifest = []byte(`{"copy":{}}`)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "module", "copytest", moduleManifestFilename), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "internal", "model", "copytest", "copytest.go"), []byte(`package modelcopytest

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type CopyTest struct {
	model.Empty
}

type CopyTestListRsp struct{}

func (CopyTest) Design() {
	dsl.Route("copytest", func() {
		dsl.Create(func() {
			dsl.Service()
			dsl.Filename("create.go")
		})
		dsl.List(func() {
			dsl.Service()
			dsl.Filename("list.go")
			dsl.Result[*CopyTestListRsp]()
		})
		dsl.Get(func() {
			dsl.Service()
			dsl.Filename("get.go")
		})
	})
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceServiceDir := filepath.Join(frameworkRoot, "internal", "service", "copytest")
	write := func(name string, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(sourceServiceDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("create.go", `package servicecopytest

import "github.com/hydroan/gst/service"

type Creator struct {
	service.Base[any, any, any]
}

func (c *Creator) Create() string {
	return createHelper() + sharedHelper()
}
`)
	write("list.go", `package servicecopytest

import "github.com/hydroan/gst/service"

type Lister struct {
	service.Base[any, any, any]
}

func (l *Lister) List() string {
	return listHelper()
}
`)
	write("get.go", `package servicecopytest

import "github.com/hydroan/gst/service"

type Getter struct {
	service.Base[any, any, any]
}

func (g *Getter) Get() string {
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
}

func moduleCopyPlanFileContent(t *testing.T, plan *CopyPlan, targetPath string) string {
	t.Helper()
	for _, file := range plan.Files {
		if file.TargetPath == targetPath {
			return string(file.Content)
		}
	}
	t.Fatalf("copy plan missing target %s", targetPath)
	return ""
}
