package ggmodule

import (
	"os"
	"path/filepath"
	"slices"
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

func TestMergeModuleServiceSourceCopiesWholeServiceFile(t *testing.T) {
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
	// Keep source method body comments.
	fmt.Println(helperValue)
	fmt.Println(s.describe("bind"))
	return &modelmfa.TOTPBindRsp{}, nil
}

// CreateAfter copies source hook logic.
func (s *TOTPBindService) CreateAfter(ctx *types.ServiceContext, req *modelmfa.TOTPBind) error {
	// Keep source hook body comments.
	fmt.Println(s.describe("after"))
	return nil
}

// describe copies source receiver helpers.
func (s *TOTPBindService) describe(step string) string {
	return helperValue + ":" + step
}

// packageHelper copies ordinary package functions.
func packageHelper() string {
	return helperValue
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
	log := t.WithContext(ctx, ctx.Phase())
	log.Info("mfa: totp bind")
	return rsp, nil
}

func (t *TotpBind) CreateAfter(ctx *types.ServiceContext, req *mfa.MFA) error {
	log := t.WithContext(ctx, ctx.Phase())
	log.Info("mfa: totp bind after")
	return nil
}
`)

	got, err := mergeModuleServiceSource(moduleServiceMergeInput{
		SourcePath:            "totp_bind.go",
		Source:                source,
		TargetPath:            "service/mfa/totp_bind.go",
		Target:                target,
		ModuleName:            "mfa",
		TargetModelImportPath: "dice/model/mfa",
	})
	if err != nil {
		t.Fatalf("mergeModuleServiceSource() error = %v", err)
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
	if !strings.Contains(code, "// Keep source method body comments.") {
		t.Fatalf("source method body comment was not copied:\n%s", code)
	}
	if !strings.Contains(code, "// CreateAfter copies source hook logic.\nfunc (t *TotpBind) CreateAfter") {
		t.Fatalf("source hook method was not copied onto target receiver:\n%s", code)
	}
	if !strings.Contains(code, "// Keep source hook body comments.") {
		t.Fatalf("source hook body comment was not copied:\n%s", code)
	}
	if !strings.Contains(code, "// describe copies source receiver helpers.\nfunc (s *TotpBind) describe(step string) string") {
		t.Fatalf("source receiver helper was not copied onto target receiver:\n%s", code)
	}
	if !strings.Contains(code, "// packageHelper copies ordinary package functions.\nfunc packageHelper() string") {
		t.Fatalf("ordinary package function comment was not copied:\n%s", code)
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

func TestMergeModuleServiceSourceAllowsHookOnlySource(t *testing.T) {
	source := []byte(`package serviceauthz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// MenuService filters menus after the built-in list flow.
type MenuService struct {
	service.Base[*modelauthz.Menu, *modelauthz.Menu, *modelauthz.Menu]
}

// ListAfter copies hook-only service logic.
func (m *MenuService) ListAfter(ctx *types.ServiceContext, data *[]*modelauthz.Menu) error {
	// Keep hook-only body comments.
	return m.filterByRole(ctx, data)
}

// filterByRole copies hook helper methods.
func (m *MenuService) filterByRole(ctx *types.ServiceContext, data *[]*modelauthz.Menu) error {
	return nil
}
`)
	target := []byte(`package authz

import (
	"dice/model/authz"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Menu struct {
	service.Base[*authz.Authz, *authz.Authz, *authz.Authz]
}

func (m *Menu) List(ctx *types.ServiceContext, req *authz.Authz) (rsp *authz.Authz, err error) {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("authz: menu")
	return rsp, nil
}
`)

	got, err := mergeModuleServiceSource(moduleServiceMergeInput{
		SourcePath:            "menu.go",
		Source:                source,
		TargetPath:            "service/authz/menu.go",
		Target:                target,
		ModuleName:            "authz",
		TargetModelImportPath: "dice/model/authz",
	})
	if err != nil {
		t.Fatalf("mergeModuleServiceSource() error = %v", err)
	}
	code := string(got)

	if !strings.Contains(code, "func (m *Menu) List(ctx *types.ServiceContext, req *authz.Authz) (rsp *authz.Authz, err error)") {
		t.Fatalf("target list method was not preserved:\n%s", code)
	}
	if !strings.Contains(code, "// ListAfter copies hook-only service logic.\nfunc (m *Menu) ListAfter") {
		t.Fatalf("hook-only method was not copied:\n%s", code)
	}
	if !strings.Contains(code, "// Keep hook-only body comments.") {
		t.Fatalf("hook-only body comment was not copied:\n%s", code)
	}
	if !strings.Contains(code, "// filterByRole copies hook helper methods.\nfunc (m *Menu) filterByRole") {
		t.Fatalf("hook helper method was not copied:\n%s", code)
	}
	if strings.Contains(code, "modelauthz") || strings.Contains(code, "MenuService") {
		t.Fatalf("source package artifacts leaked into target:\n%s", code)
	}
}

func TestMergeModuleServiceSourceRetargetsMethodBodyParameterNames(t *testing.T) {
	source := []byte(`package serviceauthz

import (
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type UserRoleService struct {
	service.Base[*modelauthz.UserRole, *modelauthz.UserRole, *modelauthz.UserRole]
}

func (s *UserRoleService) ListAfter(ctx *types.ServiceContext, data *[]*modelauthz.UserRole) error {
	for _, ur := range *data {
		_ = ur
	}
	return nil
}
`)
	target := []byte(`package authz

import (
	"dice/model/authz"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type UserRole struct {
	service.Base[*authz.UserRole, *authz.UserRole, *authz.UserRole]
}

func (u *UserRole) ListAfter(ctx *types.ServiceContext, userroles *[]*authz.UserRole) error {
	return nil
}
`)

	got, err := mergeModuleServiceSource(moduleServiceMergeInput{
		SourcePath:            "user_role.go",
		Source:                source,
		TargetPath:            "service/authz/user_role.go",
		Target:                target,
		ModuleName:            "authz",
		TargetModelImportPath: "dice/model/authz",
	})
	if err != nil {
		t.Fatalf("mergeModuleServiceSource() error = %v", err)
	}
	code := string(got)
	if !strings.Contains(code, "func (u *UserRole) ListAfter(ctx *types.ServiceContext, userroles *[]*authz.UserRole) error") {
		t.Fatalf("target method signature was not preserved:\n%s", code)
	}
	if !strings.Contains(code, "for _, ur := range *userroles") {
		t.Fatalf("source body parameter reference was not retargeted:\n%s", code)
	}
	if strings.Contains(code, "*data") {
		t.Fatalf("source parameter name leaked into target body:\n%s", code)
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

func (s *CustomService) ListAfter(ctx *types.ServiceContext, data *[]any) error {
	return nil
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
		ModulePath:    "tmpapp",
		ModelFileDir:  filepath.Join("model", "copytest"),
		ModelFilePath: filepath.Join("model", "copytest", "copytest.go"),
		ModelPkgName:  "copytest",
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
}

func TestCollectActionsAllowsMultipleModelDesigns(t *testing.T) {
	sourceServiceDir := t.TempDir()
	for name, source := range map[string]string{
		"role.go": `package servicecopytest

import "github.com/hydroan/gst/service"

type RoleService struct {
	service.Base[any, any, any]
}
`,
		"menu.go": `package servicecopytest

import "github.com/hydroan/gst/service"

type MenuService struct {
	service.Base[any, any, any]
}
`,
	} {
		if err := os.WriteFile(filepath.Join(sourceServiceDir, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	plan := &CopyPlan{
		Name:              "copytest",
		ProjectModulePath: "tmpapp",
		SourceServiceDir:  sourceServiceDir,
		TargetModelDir:    filepath.Join("model", "copytest"),
		TargetServiceDir:  filepath.Join("service", "copytest"),
	}
	models := []*gen.ModelInfo{
		{
			ModulePath:    "tmpapp",
			ModelFileDir:  filepath.Join("model", "copytest"),
			ModelFilePath: filepath.Join("model", "copytest", "role.go"),
			ModelPkgName:  "copytest",
			ModelName:     "Role",
			ModelVarName:  "r",
			Design: &dsl.Design{
				Enabled:    true,
				Endpoint:   "roles",
				Create:     &dsl.Action{Enabled: true, Service: true, Filename: "role.go", Phase: consts.PHASE_CREATE},
				Delete:     &dsl.Action{},
				Update:     &dsl.Action{},
				Patch:      &dsl.Action{},
				List:       &dsl.Action{},
				Get:        &dsl.Action{},
				CreateMany: &dsl.Action{},
				DeleteMany: &dsl.Action{},
				UpdateMany: &dsl.Action{},
				PatchMany:  &dsl.Action{},
				Import:     &dsl.Action{},
				Export:     &dsl.Action{},
			},
		},
		{
			ModulePath:    "tmpapp",
			ModelFileDir:  filepath.Join("model", "copytest"),
			ModelFilePath: filepath.Join("model", "copytest", "menu.go"),
			ModelPkgName:  "copytest",
			ModelName:     "Menu",
			ModelVarName:  "m",
			Design: &dsl.Design{
				Enabled:    true,
				Endpoint:   "menus",
				Create:     &dsl.Action{},
				Delete:     &dsl.Action{},
				Update:     &dsl.Action{},
				Patch:      &dsl.Action{},
				List:       &dsl.Action{Enabled: true, Service: true, Filename: "menu.go", Phase: consts.PHASE_LIST},
				Get:        &dsl.Action{},
				CreateMany: &dsl.Action{},
				DeleteMany: &dsl.Action{},
				UpdateMany: &dsl.Action{},
				PatchMany:  &dsl.Action{},
				Import:     &dsl.Action{},
				Export:     &dsl.Action{},
			},
		},
	}

	actions, err := plan.collectActions(models)
	if err != nil {
		t.Fatalf("collectActions() error = %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("collectActions() returned %d actions, want 2: %#v", len(actions), actions)
	}

	gotModelsByFile := make(map[string]string)
	for _, action := range actions {
		gotModelsByFile[filepath.Base(action.TargetPath)] = action.ModelInfo.ModelName
	}
	if gotModelsByFile["role.go"] != "Role" {
		t.Fatalf("role.go action model = %q, want Role", gotModelsByFile["role.go"])
	}
	if gotModelsByFile["menu.go"] != "Menu" {
		t.Fatalf("menu.go action model = %q, want Menu", gotModelsByFile["menu.go"])
	}
}

func TestAddServiceFilesMergesActionsSharingServiceFile(t *testing.T) {
	sourceServiceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceServiceDir, "role.go"), []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// RoleService owns role hooks.
type RoleService struct {
	service.Base[*modelcopytest.CopyTest, *modelcopytest.CopyTest, *modelcopytest.CopyTest]
}

// CreateAfter copies create hook logic.
func (s *RoleService) CreateAfter(ctx *types.ServiceContext, req *modelcopytest.CopyTest) error {
	return nil
}

// DeleteAfter copies delete hook logic.
func (s *RoleService) DeleteAfter(ctx *types.ServiceContext, req *modelcopytest.CopyTest) error {
	return nil
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	modelInfo := &gen.ModelInfo{
		ModulePath:    "tmpapp",
		ModelFileDir:  filepath.Join("model", "copytest"),
		ModelFilePath: filepath.Join("model", "copytest", "copytest.go"),
		ModelPkgName:  "copytest",
		ModelName:     "CopyTest",
		ModelVarName:  "c",
		Design:        &dsl.Design{Enabled: true},
	}
	createAction := &dsl.Action{
		Enabled:  true,
		Service:  true,
		Filename: "role.go",
		Payload:  "*CopyTest",
		Result:   "*CopyTest",
		Phase:    consts.PHASE_CREATE,
	}
	deleteAction := &dsl.Action{
		Enabled:  true,
		Service:  true,
		Filename: "role.go",
		Payload:  "*CopyTest",
		Result:   "*CopyTest",
		Phase:    consts.PHASE_DELETE,
	}
	plan := &CopyPlan{
		Name:                  "copytest",
		ProjectModulePath:     "tmpapp",
		SourceServiceDir:      sourceServiceDir,
		TargetServiceDir:      filepath.Join("service", "copytest"),
		TargetModelImportPath: filepath.Join("tmpapp", "model", "copytest"),
		Actions: []moduleCopyAction{
			{
				Action:     createAction,
				SourcePath: filepath.Join(sourceServiceDir, "role.go"),
				TargetPath: filepath.Join("service", "copytest", "role.go"),
				ModelInfo:  modelInfo,
			},
			{
				Action:     deleteAction,
				SourcePath: filepath.Join(sourceServiceDir, "role.go"),
				TargetPath: filepath.Join("service", "copytest", "role.go"),
				ModelInfo:  modelInfo,
			},
		},
	}

	if err := plan.addServiceFiles(nil); err != nil {
		t.Fatalf("addServiceFiles() error = %v", err)
	}
	targets := plan.ServiceTargets()
	if len(targets) != 1 {
		t.Fatalf("ServiceTargets() = %v, want one merged role.go target", targets)
	}
	code := string(plan.Files[0].Content)
	for _, want := range []string{
		"func (r *Role) Create(ctx *types.ServiceContext, req *copytest.CopyTest) (rsp *copytest.CopyTest, err error)",
		"func (r *Role) Delete(ctx *types.ServiceContext, req *copytest.CopyTest) (rsp *copytest.CopyTest, err error)",
		"// CreateAfter copies create hook logic.\nfunc (r *Role) CreateAfter",
		"// DeleteAfter copies delete hook logic.\nfunc (r *Role) DeleteAfter",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("merged service file missing %q:\n%s", want, code)
		}
	}
}

func TestAddServiceFilesUsesFlattenServicePackage(t *testing.T) {
	sourceServiceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceServiceDir, "role.go"), []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type RoleService struct {
	service.Base[*modelcopytest.Role, *modelcopytest.Role, *modelcopytest.Role]
}

func (s *RoleService) CreateAfter(ctx *types.ServiceContext, req *modelcopytest.Role) error {
	return nil
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	modelInfo := &gen.ModelInfo{
		ModulePath:    "tmpapp",
		ModelFileDir:  filepath.Join("model", "copytest"),
		ModelFilePath: filepath.Join("model", "copytest", "role.go"),
		ModelPkgName:  "copytest",
		ModelName:     "Role",
		ModelVarName:  "r",
		Design:        &dsl.Design{Enabled: true},
	}
	createAction := &dsl.Action{
		Enabled:  true,
		Service:  true,
		Filename: "role.go",
		Flatten:  true,
		Payload:  "*Role",
		Result:   "*Role",
		Phase:    consts.PHASE_CREATE,
	}
	plan := &CopyPlan{
		Name:                  "copytest",
		ProjectModulePath:     "tmpapp",
		SourceServiceDir:      sourceServiceDir,
		TargetServiceDir:      filepath.Join("service", "copytest"),
		TargetModelImportPath: filepath.Join("tmpapp", "model", "copytest"),
		Actions: []moduleCopyAction{
			{
				Action:     createAction,
				SourcePath: filepath.Join(sourceServiceDir, "role.go"),
				TargetPath: filepath.Join("service", "copytest", "role.go"),
				ModelInfo:  modelInfo,
			},
		},
	}

	if err := plan.addServiceFiles(nil); err != nil {
		t.Fatalf("addServiceFiles() error = %v", err)
	}
	code := string(plan.Files[0].Content)
	if !strings.HasPrefix(code, "package copytest\n") {
		t.Fatalf("flattened merged service package mismatch:\n%s", code)
	}
	if strings.HasPrefix(code, "package role\n") {
		t.Fatalf("flattened merged service kept role package:\n%s", code)
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

func TestBuildModuleCopyPlanIncludesAuthzMiddlewareFiles(t *testing.T) {
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

	plan, err := BuildCopyPlan("authz", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	targets := plan.MiddlewareTargets()
	if !slices.Contains(targets, filepath.Join("middleware", "authz.go")) {
		t.Fatalf("MiddlewareTargets() = %v, want middleware/authz.go", targets)
	}
}

func TestCopyExecutionCopiesMiddlewareAndRegistersAuth(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "middleware"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "middleware", "middleware.go"), []byte(`package middleware

func init() {
	// keep existing comments
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(projectDir)

	source := []byte(`package middleware

func Authz() any {
	return nil
}
`)
	plan := &CopyPlan{
		Name:                "authz",
		ModelDir:            "model",
		ServiceDir:          "service",
		TargetMiddlewareDir: "middleware",
		Files: []moduleCopyFile{
			{
				Kind:       moduleCopyFileMiddleware,
				TargetPath: filepath.Join("middleware", "authz.go"),
				Content:    source,
			},
		},
		Middleware: []moduleCopyMiddleware{
			{
				SourcePath: filepath.Join("internal", "gst", "middleware", "authz.go"),
				TargetPath: filepath.Join("middleware", "authz.go"),
				Scope:      moduleCopyMiddlewareScopeAuth,
				Handler:    "Authz",
			},
		},
	}
	exec := &CopyExecution{
		Plan:    plan,
		Options: CopyOptions{},
		RunGen: func() error {
			return nil
		},
	}

	if err := exec.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	copied, err := os.ReadFile(filepath.Join(projectDir, "middleware", "authz.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != string(source) {
		t.Fatalf("copied middleware source changed:\n%s", copied)
	}

	registered, err := os.ReadFile(filepath.Join(projectDir, "middleware", "middleware.go"))
	if err != nil {
		t.Fatal(err)
	}
	code := string(registered)
	if !strings.Contains(code, `"github.com/hydroan/gst/middleware"`) {
		t.Fatalf("middleware registration import missing:\n%s", code)
	}
	if !strings.Contains(code, "middleware.RegisterAuth(Authz())") {
		t.Fatalf("auth middleware registration missing:\n%s", code)
	}
	if strings.Contains(code, "gstmiddleware") {
		t.Fatalf("middleware registration used an unnecessary alias:\n%s", code)
	}
}

func TestBuildModuleCopyPlanReportsExtraTargetModelFiles(t *testing.T) {
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

	targetModelDir := filepath.Join(projectDir, "model", "authz")
	if mkdirErr := os.MkdirAll(targetModelDir, 0o755); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	extraTarget := filepath.Join(targetModelDir, "design.go")
	if writeErr := os.WriteFile(extraTarget, []byte("package authz\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(targetModelDir, "design_test.go"), []byte("package authz\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("authz", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	extraTargets := plan.ExtraModelTargets()
	if len(extraTargets) != 1 {
		t.Fatalf("ExtraModelTargets() = %v, want one extra target", extraTargets)
	}
	want := filepath.Join("model", "authz", "design.go")
	if extraTargets[0] != want {
		t.Fatalf("ExtraModelTargets()[0] = %q, want %q", extraTargets[0], want)
	}
}

func TestBuildModuleCopyPlanReportsExtraTargetServiceFiles(t *testing.T) {
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

	targetServiceDir := filepath.Join(projectDir, "service", "mfa")
	if mkdirErr := os.MkdirAll(targetServiceDir, 0o755); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	extraTarget := filepath.Join(targetServiceDir, "user_authenticator.go")
	if writeErr := os.WriteFile(extraTarget, []byte("package mfa\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(targetServiceDir, "account_authenticator_test.go"), []byte("package mfa\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("mfa", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	extraTargets := plan.ExtraServiceTargets()
	if len(extraTargets) != 1 {
		t.Fatalf("ExtraServiceTargets() = %v, want one extra target", extraTargets)
	}
	want := filepath.Join("service", "mfa", "user_authenticator.go")
	if extraTargets[0] != want {
		t.Fatalf("ExtraServiceTargets()[0] = %q, want %q", extraTargets[0], want)
	}
}

func TestBuildModuleCopyPlanIgnoresFrameworkRootRelativeFiles(t *testing.T) {
	projectDir := t.TempDir()
	frameworkRoot := filepath.Join(projectDir, "internal", "gst")
	for _, dir := range []string{
		filepath.Join(frameworkRoot, "module", "copytest"),
		filepath.Join(frameworkRoot, "internal", "model", "copytest"),
		filepath.Join(frameworkRoot, "internal", "service", "copytest"),
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
	if err := os.WriteFile(filepath.Join(frameworkRoot, "module", "copytest", moduleManifestFilename), []byte(`{
		"copy": {
			"excludeSourceFiles": ["internal/model/copytest/ignored.go"]
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "internal", "service", "copytest", "service.go"), []byte("package servicecopytest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "internal", "model", "copytest", "kept.go"), []byte(`package modelcopytest

import "github.com/hydroan/gst/model"

type Kept struct {
	model.Empty
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "internal", "model", "copytest", "ignored.go"), []byte(`package modelcopytest

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Ignored struct {
	model.Empty
}

func (Ignored) Design() {
	dsl.Create(func() {
		dsl.Service(true)
		dsl.Filename("missing.go")
	})
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	targets := plan.ModelTargets()
	if !slices.Contains(targets, filepath.Join("model", "copytest", "kept.go")) {
		t.Fatalf("ModelTargets() = %v, want kept.go", targets)
	}
	if slices.Contains(targets, filepath.Join("model", "copytest", "ignored.go")) {
		t.Fatalf("ModelTargets() = %v, ignored.go should not be copied", targets)
	}
	if len(plan.ServiceTargets()) != 0 {
		t.Fatalf("ServiceTargets() = %v, ignored model action should not be collected", plan.ServiceTargets())
	}
}
