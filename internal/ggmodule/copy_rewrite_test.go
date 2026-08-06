package ggmodule

import (
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
