package ggmodule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/types/consts"
)

func TestValidateModuleCopyNameRejectsPaths(t *testing.T) {
	tests := []string{
		"module/mfa",
		"./mfa",
		"../mfa",
		`module\mfa`,
		".mfa",
		"",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateModuleCopyName(name); err == nil {
				t.Fatalf("validateModuleCopyName(%q) succeeded, want error", name)
			}
		})
	}

	if err := validateModuleCopyName("mfa"); err != nil {
		t.Fatalf("validateModuleCopyName(%q) = %v, want nil", "mfa", err)
	}
}

func TestNormalizeModuleModelSourceUsesTargetPackage(t *testing.T) {
	src := []byte(`// Package modelmfa contains MFA models.
package modelmfa

import "github.com/hydroan/gst/model"

type MFA struct {
	model.Empty
}
`)

	got, err := normalizeModuleModelSource("mfa.go", src, "mfa")
	if err != nil {
		t.Fatalf("normalizeModuleModelSource() error = %v", err)
	}
	if !strings.Contains(string(got), "package mfa") {
		t.Fatalf("normalized source missing target package:\n%s", got)
	}
	if strings.Contains(string(got), "package modelmfa") {
		t.Fatalf("normalized source kept source package:\n%s", got)
	}
}

func TestMergeModuleActionServiceSourceKeepsTargetSignature(t *testing.T) {
	source := []byte(`package servicemfa

import (
	"fmt"

	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

const helperValue = "copied"

// TOTPBindService starts the source binding flow.
//
// It should be copied to the target service struct comment.
type TOTPBindService struct {
	service.Base[*modelmfa.TOTPBind, *modelmfa.TOTPBind, *modelmfa.TOTPBindRsp]
}

// Create copies the source business logic.
func (s *TOTPBindService) Create(ctx *types.ServiceContext, req *modelmfa.TOTPBind) (rsp *modelmfa.TOTPBindRsp, err error) {
	fmt.Println(helperValue)
	return &modelmfa.TOTPBindRsp{}, nil
}
`)
	target := []byte(`package mfa

import (
	"dice/model/mfa"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type TotpBind struct {
	service.Base[*mfa.MFA, *mfa.MFA, *mfa.TOTPBindRsp]
}

func (t *TotpBind) Create(ctx *types.ServiceContext, req *mfa.MFA) (rsp *mfa.TOTPBindRsp, err error) {
	log := t.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("mfa: totp bind")
	return rsp, nil
}
`)

	got, err := mergeModuleActionServiceSource(moduleActionMergeInput{
		SourcePath:            "totp_bind.go",
		Source:                source,
		TargetPath:            "service/mfa/totp_bind.go",
		Target:                target,
		ModuleName:            "mfa",
		TargetModelImportPath: "dice/model/mfa",
		MethodName:            "Create",
	})
	if err != nil {
		t.Fatalf("mergeModuleActionServiceSource() error = %v", err)
	}
	code := string(got)

	if !strings.Contains(code, "func (t *TotpBind) Create(ctx *types.ServiceContext, req *mfa.MFA) (rsp *mfa.TOTPBindRsp, err error)") {
		t.Fatalf("target signature was not preserved:\n%s", code)
	}
	if !strings.Contains(code, "// TotpBind starts the source binding flow.") {
		t.Fatalf("source service struct doc was not copied and retargeted:\n%s", code)
	}
	if !strings.Contains(code, "// TotpBind starts the source binding flow.\n//\n// It should be copied to the target service struct comment.\ntype TotpBind struct") {
		t.Fatalf("source service struct doc was not placed before target struct:\n%s", code)
	}
	if !strings.Contains(code, "// Create copies the source business logic.") {
		t.Fatalf("source method doc was not copied:\n%s", code)
	}
	if !strings.Contains(code, "// Create copies the source business logic.\nfunc (t *TotpBind) Create") {
		t.Fatalf("source method doc was not placed before target method:\n%s", code)
	}
	if !strings.Contains(code, `const helperValue = "copied"`) {
		t.Fatalf("ordinary source declaration was not copied:\n%s", code)
	}
	if !strings.Contains(code, "return &mfa.TOTPBindRsp{}, nil") {
		t.Fatalf("source model selector was not rewritten:\n%s", code)
	}
	if strings.Contains(code, "modelmfa") || strings.Contains(code, "TOTPBindService") {
		t.Fatalf("source package artifacts leaked into target:\n%s", code)
	}
}

func TestCollectActionsIgnoresActionsWithoutService(t *testing.T) {
	sourceServiceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceServiceDir, "custom.go"), []byte(`package servicecopytest

import (
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type CustomService struct {
	service.Base[any, any, any]
}

func (s *CustomService) List(ctx *types.ServiceContext, req any) (rsp any, err error) {
	return nil, nil
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	plan := &CopyPlan{
		Name:              "copytest",
		ProjectModulePath: "tmpapp",
		SourceServiceDir:  sourceServiceDir,
		TargetModelDir:    filepath.Join("model", "copytest"),
		TargetServiceDir:  filepath.Join("service", "copytest"),
	}
	modelInfo := &gen.ModelInfo{
		ModulePath:    frameworkModulePath,
		ModelFileDir:  filepath.Join("internal", "model", "copytest"),
		ModelFilePath: filepath.Join("internal", "model", "copytest", "copytest.go"),
		ModelPkgName:  "modelcopytest",
		ModelName:     "CopyTest",
		ModelVarName:  "c",
		Design: &dsl.Design{
			Enabled:    true,
			Endpoint:   "copytest",
			Create:     &dsl.Action{Enabled: true, Phase: consts.PHASE_CREATE},
			Delete:     &dsl.Action{},
			Update:     &dsl.Action{},
			Patch:      &dsl.Action{},
			List:       &dsl.Action{Enabled: true, Service: true, Filename: "custom.go", Phase: consts.PHASE_LIST},
			Get:        &dsl.Action{},
			CreateMany: &dsl.Action{},
			DeleteMany: &dsl.Action{},
			UpdateMany: &dsl.Action{},
			PatchMany:  &dsl.Action{},
			Import:     &dsl.Action{},
			Export:     &dsl.Action{},
		},
	}

	actions, err := plan.collectActions([]*gen.ModelInfo{modelInfo})
	if err != nil {
		t.Fatalf("collectActions() error = %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("collectActions() returned %d actions, want 1: %#v", len(actions), actions)
	}
	if got := filepath.Base(actions[0].SourcePath); got != "custom.go" {
		t.Fatalf("collected source file = %q, want custom.go", got)
	}
	if got := actions[0].MethodName; got != "List" {
		t.Fatalf("collected method = %q, want List", got)
	}
}

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

func TestModuleCopyHelperDependencyFilesFindsMFAHelpers(t *testing.T) {
	sourceServiceDir := filepath.Join("..", "service", "mfa")
	actionFile := filepath.Join(sourceServiceDir, "totp_bind.go")

	got, err := moduleCopyHelperDependencyFiles(sourceServiceDir, []string{actionFile})
	if err != nil {
		t.Fatalf("moduleCopyHelperDependencyFiles() error = %v", err)
	}

	var found bool
	for _, file := range got {
		if filepath.Base(file) == "totp_binding_challenge.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("moduleCopyHelperDependencyFiles() = %v, want totp_binding_challenge.go", got)
	}
}

func TestModuleCopyHelperDependencyFilesFindsMFAHelpersThroughSymlink(t *testing.T) {
	realRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	linkParent := t.TempDir()
	linkRoot := filepath.Join(linkParent, "gst")
	if symlinkErr := os.Symlink(realRoot, linkRoot); symlinkErr != nil {
		t.Skipf("symlink not available: %v", symlinkErr)
	}

	sourceServiceDir := filepath.Join(linkRoot, "internal", "service", "mfa")
	actionFile := filepath.Join(sourceServiceDir, "totp_bind.go")
	got, err := moduleCopyHelperDependencyFiles(sourceServiceDir, []string{actionFile})
	if err != nil {
		t.Fatalf("moduleCopyHelperDependencyFiles() error = %v", err)
	}

	var found bool
	for _, file := range got {
		if filepath.Base(file) == "totp_binding_challenge.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("moduleCopyHelperDependencyFiles() = %v, want totp_binding_challenge.go", got)
	}
}

func TestModuleCopyHelperDependencyFilesFindsMFAHelpersFromAllActions(t *testing.T) {
	sourceServiceDir := filepath.Join("..", "service", "mfa")
	actionFiles := []string{
		filepath.Join(sourceServiceDir, "totp_bind.go"),
		filepath.Join(sourceServiceDir, "totp_check.go"),
		filepath.Join(sourceServiceDir, "totp_confirm.go"),
		filepath.Join(sourceServiceDir, "totp_status.go"),
		filepath.Join(sourceServiceDir, "totp_unbind.go"),
		filepath.Join(sourceServiceDir, "totp_verify.go"),
	}

	got, err := moduleCopyHelperDependencyFiles(sourceServiceDir, actionFiles)
	if err != nil {
		t.Fatalf("moduleCopyHelperDependencyFiles() error = %v", err)
	}

	want := map[string]bool{
		"totp_backup_code.go":       false,
		"totp_binding_challenge.go": false,
		"totp_code.go":              false,
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

func TestBuildModuleCopyPlanIncludesMFAHelperFiles(t *testing.T) {
	frameworkRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	projectDir := t.TempDir()
	if mkdirErr := os.Mkdir(filepath.Join(projectDir, "internal"), 0o755); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	if symlinkErr := os.Symlink(frameworkRoot, filepath.Join(projectDir, "internal", "gst")); symlinkErr != nil {
		t.Skipf("symlink not available: %v", symlinkErr)
	}
	if writeErr := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte(`module tmpapp

go 1.26

require github.com/hydroan/gst v0.0.0

replace github.com/hydroan/gst => ./internal/gst
`), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("mfa", CopyOptions{})
	if err != nil {
		t.Fatalf("buildModuleCopyPlan() error = %v", err)
	}

	helpers := plan.HelperTargets()
	want := map[string]bool{
		filepath.Join("service", "mfa", "totp_backup_code.go"):       false,
		filepath.Join("service", "mfa", "totp_binding_challenge.go"): false,
		filepath.Join("service", "mfa", "totp_code.go"):              false,
	}
	for _, helper := range helpers {
		if _, ok := want[helper]; ok {
			want[helper] = true
		}
	}
	for helper, found := range want {
		if !found {
			t.Fatalf("plan helperTargets() = %v, want %s", helpers, helper)
		}
	}
}
