package ggmodule

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeModuleModelSourceUsesTargetPackage(t *testing.T) {
	src := []byte(`// Package modelcopytest contains copytest models.
package modelcopytest

import "github.com/hydroan/gst/model"

type CopyTest struct {
	model.Empty
}
`)

	got, err := normalizeModuleModelSource("copytest.go", src, moduleCopyRewriteConfig{
		ModuleName:        "copytest",
		ProjectModulePath: "tmpapp",
		ModelDir:          "model",
		ServiceDir:        "service",
		TargetPackage:     "copytest",
	})
	if err != nil {
		t.Fatalf("normalizeModuleModelSource() error = %v", err)
	}
	if !strings.Contains(string(got), "package copytest") {
		t.Fatalf("normalized source missing target package:\n%s", got)
	}
	if strings.Contains(string(got), "package modelcopytest") {
		t.Fatalf("normalized source kept source package:\n%s", got)
	}
}

func TestNormalizeModuleModelSourceRewritesCopiedModelImports(t *testing.T) {
	src := []byte(`package modelcopytestentry

import (
	modelcopytestsession "github.com/hydroan/gst/internal/model/copytest/session"
	"github.com/hydroan/gst/model"
)

type EntryAction struct {
	model.Empty
}

type ActionRsp = modelcopytestsession.ActionRsp
`)

	got, err := normalizeModuleModelSource("entry.go", src, moduleCopyRewriteConfig{
		ModuleName:        "copytest",
		ProjectModulePath: "tmpapp",
		ModelDir:          "model",
		ServiceDir:        "service",
		TargetPackage:     "entry",
	})
	if err != nil {
		t.Fatalf("normalizeModuleModelSource() error = %v", err)
	}
	code := string(got)
	for _, want := range []string{
		"package entry\n",
		`"tmpapp/model/copytest/session"`,
		"type ActionRsp = session.ActionRsp",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("normalized model source missing %q:\n%s", want, code)
		}
	}
	if strings.Contains(code, "github.com/hydroan/gst/internal/model/copytest/session") || strings.Contains(code, "modelcopytestsession") {
		t.Fatalf("normalized model source leaked framework model import artifacts:\n%s", code)
	}
}

func TestNormalizeModuleServiceSourceAliasesConflictingCopiedImports(t *testing.T) {
	source := []byte(`package servicecopytestentry

import (
	modelcopytestsession "github.com/hydroan/gst/internal/model/copytest/session"
	servicecopytestsession "github.com/hydroan/gst/internal/service/copytest/session"
)

func useCopiedSessionPackages() {
	_ = modelcopytestsession.Session{}
	servicecopytestsession.Touch()
}
`)

	got, err := normalizeModuleServiceSource("entry.go", source, moduleCopyRewriteConfig{
		ModuleName:        "copytest",
		ProjectModulePath: "tmpapp",
		ModelDir:          "model",
		ServiceDir:        "service",
		TargetPackage:     "entry",
	})
	if err != nil {
		t.Fatalf("normalizeModuleServiceSource() error = %v", err)
	}
	code := string(got)
	for _, want := range []string{
		"package entry\n",
		`"tmpapp/model/copytest/session"`,
		`servicesession "tmpapp/service/copytest/session"`,
		"_ = session.Session{}",
		"servicesession.Touch()",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("normalized service source missing %q:\n%s", want, code)
		}
	}
	if strings.Contains(code, "modelcopytestsession") || strings.Contains(code, "servicecopytestsession") || strings.Contains(code, "modelsession") {
		t.Fatalf("normalized service source leaked source aliases:\n%s", code)
	}
}

func TestNormalizeModuleServiceSourceRejectsDeclarationsShadowingCopiedImports(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "local variable",
			source: `package servicecopytestentry

import modelcopytestsession "github.com/hydroan/gst/internal/model/copytest/session"

func loadSession(id string) string {
	session := modelcopytestsession.Load(id)
	return modelcopytestsession.Key(session)
}
`,
		},
		{
			name: "package-level function",
			source: `package servicecopytestentry

import modelcopytestsession "github.com/hydroan/gst/internal/model/copytest/session"

func session() string {
	return modelcopytestsession.Name()
}
`,
		},
		{
			name: "function parameter",
			source: `package servicecopytestentry

import modelcopytestsession "github.com/hydroan/gst/internal/model/copytest/session"

func loadSession(session string) string {
	return modelcopytestsession.Key(session)
}
`,
		},
		{
			name: "range binding",
			source: `package servicecopytestentry

import modelcopytestsession "github.com/hydroan/gst/internal/model/copytest/session"

func loadSessions(ids []string) {
	for _, session := range ids {
		modelcopytestsession.Touch(session)
	}
}
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeModuleServiceSource("entry.go", []byte(test.source), moduleCopyRewriteConfig{
				ModuleName:        "copytest",
				ProjectModulePath: "tmpapp",
				ModelDir:          "model",
				ServiceDir:        "service",
				TargetPackage:     "entry",
			})
			if err == nil {
				t.Fatal("normalizeModuleServiceSource() error = nil, want shadowed package reference rejection")
			}
			for _, want := range []string{"entry.go", `"session"`} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("normalizeModuleServiceSource() error = %v, want it to mention %q", err, want)
				}
			}
		})
	}
}

func TestNormalizeModuleServiceSourceAllowsDeclarationsThatDoNotShadowCopiedImports(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "struct field",
			source: `package servicecopytestentry

import modelcopytestsession "github.com/hydroan/gst/internal/model/copytest/session"

type Entry struct {
	session string
}

func loadSession(entry Entry) string {
	return modelcopytestsession.Key(entry.session)
}
`,
			want: "return session.Key(entry.session)",
		},
		{
			// The declared name only reaches the end of the short variable
			// declaration, so the package reference on its right-hand side is
			// still the package.
			name: "declaration initialized from the package it shadows",
			source: `package servicecopytestentry

import modelcopytestsession "github.com/hydroan/gst/internal/model/copytest/session"

func loadSession(id string) string {
	session := modelcopytestsession.Load(id)
	return session.Name
}
`,
			want: "session := session.Load(id)",
		},
		{
			name: "declaration in a sibling scope",
			source: `package servicecopytestentry

import modelcopytestsession "github.com/hydroan/gst/internal/model/copytest/session"

func loadSession(id string) string {
	if id == "" {
		session := modelcopytestsession.Fallback()
		return session.Name
	}
	return modelcopytestsession.Key(id)
}
`,
			want: "return session.Key(id)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeModuleServiceSource("entry.go", []byte(test.source), moduleCopyRewriteConfig{
				ModuleName:        "copytest",
				ProjectModulePath: "tmpapp",
				ModelDir:          "model",
				ServiceDir:        "service",
				TargetPackage:     "entry",
			})
			if err != nil {
				t.Fatalf("normalizeModuleServiceSource() error = %v", err)
			}
			if !strings.Contains(string(got), test.want) {
				t.Fatalf("normalized service source missing %q:\n%s", test.want, got)
			}
		})
	}
}

// TestNormalizeModuleCopySourceAcceptsShippedModuleSources keeps the module tree
// itself copy-clean: a declaration shadowing a copied import only surfaces when a
// project runs gg module copy, so the framework asserts it up front instead of
// shipping a module that cannot be copied.
func TestNormalizeModuleCopySourceAcceptsShippedModuleSources(t *testing.T) {
	frameworkRoot, err := findFrameworkRoot()
	if err != nil {
		t.Fatalf("findFrameworkRoot() error = %v", err)
	}
	modules, err := ListModules()
	if err != nil {
		t.Fatalf("ListModules() error = %v", err)
	}

	for _, module := range modules {
		if !module.Copyable {
			continue
		}
		t.Run(module.Name, func(t *testing.T) {
			normalizeSourceTree(t, filepath.Join(frameworkRoot, "internal", "model", module.Name), module.Name, normalizeModuleModelSource)
			normalizeSourceTree(t, filepath.Join(frameworkRoot, "internal", "service", module.Name), module.Name, normalizeModuleServiceSource)
		})
	}
}

func normalizeSourceTree(t *testing.T, root string, moduleName string, normalize func(string, []byte, moduleCopyRewriteConfig) ([]byte, error)) {
	t.Helper()

	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := normalize(path, src, moduleCopyRewriteConfig{
			ModuleName:        moduleName,
			ProjectModulePath: "tmpapp",
			ModelDir:          "model",
			ServiceDir:        "service",
			TargetPackage:     moduleCopyPackageName(filepath.Dir(path)),
		}); err != nil {
			t.Errorf("module source is not copy-clean: %v", err)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s error = %v", root, walkErr)
	}
}

func TestNormalizeModuleCopySourceRejectsFrameworkInternalImports(t *testing.T) {
	// Inside the framework an internal import compiles, but the copied file
	// lands in a consumer project where Go forbids it; the copy must fail
	// instead of shipping a file that cannot build.
	source := []byte(`package middlewarecopytest

import (
	"context"

	"github.com/hydroan/gst/internal/requestctx"
)

func withMetadata(ctx context.Context) context.Context {
	return requestctx.WithMetadata(ctx, requestctx.Metadata{})
}
`)

	_, err := normalizeModuleMiddlewareSource("sample.go", source, moduleCopyRewriteConfig{
		ModuleName:        "copytest",
		ProjectModulePath: "tmpapp",
		ModelDir:          "model",
		ServiceDir:        "service",
		TargetPackage:     "middleware",
	})
	if err == nil {
		t.Fatal("normalizeModuleMiddlewareSource() must reject a surviving framework internal import")
	}
	for _, want := range []string{"github.com/hydroan/gst/internal/requestctx", "public"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}
