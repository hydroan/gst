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
		"module/copytest",
		"./copytest",
		"../copytest",
		`module\copytest`,
		".copytest",
		"",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateModuleCopyName(name); err == nil {
				t.Fatalf("validateModuleCopyName(%q) succeeded, want error", name)
			}
		})
	}

	if err := validateModuleCopyName("copytest"); err != nil {
		t.Fatalf("validateModuleCopyName(%q) = %v, want nil", "copytest", err)
	}
}

func TestNormalizeModuleModelSourceUsesTargetPackage(t *testing.T) {
	src := []byte(`// Package modelcopytest contains copytest models.
package modelcopytest

import "github.com/hydroan/gst/model"

type CopyTest struct {
	model.Empty
}
`)

	got, err := normalizeModuleModelSource("copytest.go", src, "copytest")
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

func TestNormalizeModuleServiceSourceAliasesConflictingCopiedImports(t *testing.T) {
	source := []byte(`package servicecopytestaccount

import (
	modelcopytestsession "github.com/hydroan/gst/internal/model/copytest/session"
	servicecopytestsession "github.com/hydroan/gst/internal/service/copytest/session"
)

func useCopiedSessionPackages() {
	_ = modelcopytestsession.Session{}
	servicecopytestsession.Touch()
}
`)

	got, err := normalizeModuleServiceSource("account.go", source, moduleCopyRewriteConfig{
		ModuleName:        "copytest",
		ProjectModulePath: "tmpapp",
		ModelDir:          "model",
		ServiceDir:        "service",
		TargetPackage:     "account",
	})
	if err != nil {
		t.Fatalf("normalizeModuleServiceSource() error = %v", err)
	}
	code := string(got)
	for _, want := range []string{
		"package account\n",
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

func TestMergeModuleServiceSourceCopiesWholeServiceFile(t *testing.T) {
	source := []byte(`package servicecopytest

import (
	"fmt"

	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

const helperValue = "copied"

// ActionService starts the source action flow.
//
// It should be copied to the target service struct comment.
type ActionService struct {
	service.Base[*modelcopytest.Action, *modelcopytest.Action, *modelcopytest.ActionRsp]
}

// Create copies the source business logic.
func (s *ActionService) Create(ctx *types.ServiceContext, req *modelcopytest.Action) (rsp *modelcopytest.ActionRsp, err error) {
	// Keep source method body comments.
	fmt.Println(helperValue)
	fmt.Println(s.describe("bind"))
	return &modelcopytest.ActionRsp{}, nil
}

// CreateAfter copies source hook logic.
func (s *ActionService) CreateAfter(ctx *types.ServiceContext, req *modelcopytest.Action) error {
	// Keep source hook body comments.
	fmt.Println(s.describe("after"))
	return nil
}

// describe copies source receiver helpers.
func (s *ActionService) describe(step string) string {
	return helperValue + ":" + step
}

// packageHelper copies ordinary package functions.
func packageHelper() string {
	return helperValue
}
`)
	target := []byte(`package copytest

import (
	"dice/model/copytest"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*copytest.CopyTest, *copytest.CopyTest, *copytest.ActionRsp]
}

func (c *Creator) Create(ctx *types.ServiceContext, req *copytest.CopyTest) (rsp *copytest.ActionRsp, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("copytest: create")
	return rsp, nil
}

func (c *Creator) CreateAfter(ctx *types.ServiceContext, req *copytest.CopyTest) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("copytest: create after")
	return nil
}
`)

	got, err := mergeModuleServiceSource(moduleServiceMergeInput{
		SourcePath:            "action.go",
		Source:                source,
		TargetPath:            "service/copytest/action.go",
		Target:                target,
		ModuleName:            "copytest",
		TargetModelImportPath: "dice/model/copytest",
	})
	if err != nil {
		t.Fatalf("mergeModuleServiceSource() error = %v", err)
	}
	code := string(got)

	if !strings.Contains(code, "func (c *Creator) Create(ctx *types.ServiceContext, req *copytest.CopyTest) (rsp *copytest.ActionRsp, err error)") {
		t.Fatalf("target signature was not preserved:\n%s", code)
	}
	if !strings.Contains(code, "// Creator starts the source action flow.") {
		t.Fatalf("source service struct doc was not copied and retargeted:\n%s", code)
	}
	if !strings.Contains(code, "// Creator starts the source action flow.\n//\n// It should be copied to the target service struct comment.\ntype Creator struct") {
		t.Fatalf("source service struct doc was not placed before target struct:\n%s", code)
	}
	if !strings.Contains(code, "// Create copies the source business logic.") {
		t.Fatalf("source method doc was not copied:\n%s", code)
	}
	if !strings.Contains(code, "// Create copies the source business logic.\nfunc (c *Creator) Create") {
		t.Fatalf("source method doc was not placed before target method:\n%s", code)
	}
	if !strings.Contains(code, "// Keep source method body comments.") {
		t.Fatalf("source method body comment was not copied:\n%s", code)
	}
	if !strings.Contains(code, "// CreateAfter copies source hook logic.\nfunc (c *Creator) CreateAfter") {
		t.Fatalf("source hook method was not copied onto target receiver:\n%s", code)
	}
	if !strings.Contains(code, "// Keep source hook body comments.") {
		t.Fatalf("source hook body comment was not copied:\n%s", code)
	}
	if !strings.Contains(code, "// describe copies source receiver helpers.\nfunc (s *Creator) describe(step string) string") {
		t.Fatalf("source receiver helper was not copied onto target receiver:\n%s", code)
	}
	if !strings.Contains(code, "// packageHelper copies ordinary package functions.\nfunc packageHelper() string") {
		t.Fatalf("ordinary package function comment was not copied:\n%s", code)
	}
	if !strings.Contains(code, `const helperValue = "copied"`) {
		t.Fatalf("ordinary source declaration was not copied:\n%s", code)
	}
	if !strings.Contains(code, "return &copytest.ActionRsp{}, nil") {
		t.Fatalf("source model selector was not rewritten:\n%s", code)
	}
	if strings.Contains(code, "modelcopytest") || strings.Contains(code, "ActionService") {
		t.Fatalf("source package artifacts leaked into target:\n%s", code)
	}
}

func TestMergeModuleServiceSourceAllowsHookOnlySource(t *testing.T) {
	source := []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ListingService filters items after the built-in list flow.
type ListingService struct {
	service.Base[*modelcopytest.CopyTest, *modelcopytest.CopyTest, *modelcopytest.CopyTest]
}

// ListAfter copies hook-only service logic.
func (l *ListingService) ListAfter(ctx *types.ServiceContext, data *[]*modelcopytest.CopyTest) error {
	// Keep hook-only body comments.
	return l.filterByOwner(ctx, data)
}

// filterByOwner copies hook helper methods.
func (l *ListingService) filterByOwner(ctx *types.ServiceContext, data *[]*modelcopytest.CopyTest) error {
	return nil
}
`)
	target := []byte(`package copytest

import (
	"dice/model/copytest"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*copytest.CopyTest, *copytest.CopyTest, *copytest.CopyTest]
}

func (l *Lister) List(ctx *types.ServiceContext, req *copytest.CopyTest) (rsp *copytest.CopyTest, err error) {
	log := l.WithContext(ctx, ctx.Phase())
	log.Info("copytest: list")
	return rsp, nil
}
`)

	got, err := mergeModuleServiceSource(moduleServiceMergeInput{
		SourcePath:            "list.go",
		Source:                source,
		TargetPath:            "service/copytest/list.go",
		Target:                target,
		ModuleName:            "copytest",
		TargetModelImportPath: "dice/model/copytest",
	})
	if err != nil {
		t.Fatalf("mergeModuleServiceSource() error = %v", err)
	}
	code := string(got)

	if !strings.Contains(code, "func (l *Lister) List(ctx *types.ServiceContext, req *copytest.CopyTest) (rsp *copytest.CopyTest, err error)") {
		t.Fatalf("target list method was not preserved:\n%s", code)
	}
	if !strings.Contains(code, "// ListAfter copies hook-only service logic.\nfunc (l *Lister) ListAfter") {
		t.Fatalf("hook-only method was not copied:\n%s", code)
	}
	if !strings.Contains(code, "// Keep hook-only body comments.") {
		t.Fatalf("hook-only body comment was not copied:\n%s", code)
	}
	if !strings.Contains(code, "// filterByOwner copies hook helper methods.\nfunc (l *Lister) filterByOwner") {
		t.Fatalf("hook helper method was not copied:\n%s", code)
	}
	if strings.Contains(code, "modelcopytest") || strings.Contains(code, "ListingService") {
		t.Fatalf("source package artifacts leaked into target:\n%s", code)
	}
}

func TestMergeModuleServiceSourceRetargetsMethodBodyParameterNames(t *testing.T) {
	source := []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type BindingService struct {
	service.Base[*modelcopytest.Binding, *modelcopytest.Binding, *modelcopytest.Binding]
}

func (s *BindingService) ListAfter(ctx *types.ServiceContext, data *[]*modelcopytest.Binding) error {
	for _, binding := range *data {
		_ = binding
	}
	return nil
}
`)
	target := []byte(`package copytest

import (
	"dice/model/copytest"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Binding struct {
	service.Base[*copytest.Binding, *copytest.Binding, *copytest.Binding]
}

func (b *Binding) ListAfter(ctx *types.ServiceContext, bindings *[]*copytest.Binding) error {
	return nil
}
`)

	got, err := mergeModuleServiceSource(moduleServiceMergeInput{
		SourcePath:            "binding.go",
		Source:                source,
		TargetPath:            "service/copytest/binding.go",
		Target:                target,
		ModuleName:            "copytest",
		TargetModelImportPath: "dice/model/copytest",
	})
	if err != nil {
		t.Fatalf("mergeModuleServiceSource() error = %v", err)
	}
	code := string(got)
	if !strings.Contains(code, "func (b *Binding) ListAfter(ctx *types.ServiceContext, bindings *[]*copytest.Binding) error") {
		t.Fatalf("target method signature was not preserved:\n%s", code)
	}
	if !strings.Contains(code, "for _, binding := range *bindings") {
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

func TestAddServiceFilesMergesActionsFromMultipleSourceServiceStructs(t *testing.T) {
	sourceServiceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceServiceDir, "profile.go"), []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ProfileGetService handles reads.
type ProfileGetService struct {
	service.Base[*modelcopytest.Profile, *modelcopytest.ProfileGetReq, *modelcopytest.ProfileGetRsp]
}

// ProfilePatchService handles writes.
type ProfilePatchService struct {
	service.Base[*modelcopytest.Profile, *modelcopytest.ProfilePatchReq, *modelcopytest.ProfilePatchRsp]
}

// Get copies get logic.
func (s *ProfileGetService) Get(ctx *types.ServiceContext, req *modelcopytest.ProfileGetReq) (rsp *modelcopytest.ProfileGetRsp, err error) {
	return profileGetResult(), nil
}

// Patch copies patch logic.
func (s *ProfilePatchService) Patch(ctx *types.ServiceContext, req *modelcopytest.ProfilePatchReq) (rsp *modelcopytest.ProfilePatchRsp, err error) {
	return profilePatchResult(), nil
}

func profileGetResult() *modelcopytest.Profile {
	return nil
}

func profilePatchResult() *modelcopytest.Profile {
	return nil
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	modelInfo := &gen.ModelInfo{
		ModulePath:    "tmpapp",
		ModelFileDir:  filepath.Join("model", "copytest"),
		ModelFilePath: filepath.Join("model", "copytest", "profile.go"),
		ModelPkgName:  "copytest",
		ModelName:     "Profile",
		ModelVarName:  "p",
		Design:        &dsl.Design{Enabled: true},
	}
	getAction := &dsl.Action{
		Enabled:  true,
		Service:  true,
		Filename: "profile.go",
		Flatten:  true,
		Payload:  "*ProfileGetReq",
		Result:   "*ProfileGetRsp",
		Phase:    consts.PHASE_GET,
	}
	patchAction := &dsl.Action{
		Enabled:  true,
		Service:  true,
		Filename: "profile.go",
		Flatten:  true,
		Payload:  "*ProfilePatchReq",
		Result:   "*ProfilePatchRsp",
		Phase:    consts.PHASE_PATCH,
	}
	plan := &CopyPlan{
		Name:                  "copytest",
		ProjectModulePath:     "tmpapp",
		SourceServiceDir:      sourceServiceDir,
		TargetServiceDir:      filepath.Join("service", "copytest"),
		TargetModelImportPath: filepath.Join("tmpapp", "model", "copytest"),
		Actions: []moduleCopyAction{
			{
				Action:     getAction,
				SourcePath: filepath.Join(sourceServiceDir, "profile.go"),
				TargetPath: filepath.Join("service", "copytest", "profile.go"),
				ModelInfo:  modelInfo,
			},
			{
				Action:     patchAction,
				SourcePath: filepath.Join(sourceServiceDir, "profile.go"),
				TargetPath: filepath.Join("service", "copytest", "profile.go"),
				ModelInfo:  modelInfo,
			},
		},
	}

	for _, action := range plan.Actions {
		if err := requireServiceSourceFile(action); err != nil {
			t.Fatalf("requireServiceSourceFile() error = %v", err)
		}
	}
	if err := plan.addServiceFiles(nil); err != nil {
		t.Fatalf("addServiceFiles() error = %v", err)
	}
	code := string(plan.Files[0].Content)
	for _, want := range []string{
		"// Get copies get logic.\nfunc (p *Profile) Get",
		"return profileGetResult(), nil",
		"// Patch copies patch logic.\nfunc (p *Profile) Patch",
		"return profilePatchResult(), nil",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("merged service file missing %q:\n%s", want, code)
		}
	}
	for _, unwanted := range []string{
		"type ProfileGetService struct",
		"type ProfilePatchService struct",
		"func (s *ProfileGetService)",
		"func (s *ProfilePatchService)",
	} {
		if strings.Contains(code, unwanted) {
			t.Fatalf("merged service file kept source service artifact %q:\n%s", unwanted, code)
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
	write("bind.go", `package servicecopytest

func Bind() string {
	return bindingChallenge() + verificationCode()
}
`)
	write("check.go", `package servicecopytest

func Check() string {
	return backupCode()
}
`)
	write("confirm.go", `package servicecopytest

func Confirm() string {
	return verificationCode()
}
`)
	write("binding_challenge.go", `package servicecopytest

func bindingChallenge() string {
	return "challenge"
}
`)
	write("backup_code.go", `package servicecopytest

func backupCode() string {
	return "backup"
}
`)
	write("verification_code.go", `package servicecopytest

func verificationCode() string {
	return "code"
}
`)
	return sourceServiceDir
}

func TestModuleCopyHelperDependencyFilesFindsServiceHelpers(t *testing.T) {
	sourceServiceDir := writeCopyTestServiceDependencyFiles(t)
	actionFile := filepath.Join(sourceServiceDir, "bind.go")

	got, err := moduleCopyHelperDependencyFiles(sourceServiceDir, []string{actionFile})
	if err != nil {
		t.Fatalf("moduleCopyHelperDependencyFiles() error = %v", err)
	}

	var found bool
	for _, file := range got {
		if filepath.Base(file) == "binding_challenge.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("moduleCopyHelperDependencyFiles() = %v, want binding_challenge.go", got)
	}
}

func TestModuleCopyHelperDependencyFilesFindsServiceHelpersThroughSymlink(t *testing.T) {
	sourceServiceDir := writeCopyTestServiceDependencyFiles(t)
	linkParent := t.TempDir()
	linkRoot := filepath.Join(linkParent, "servicecopytest")
	if symlinkErr := os.Symlink(sourceServiceDir, linkRoot); symlinkErr != nil {
		t.Skipf("symlink not available: %v", symlinkErr)
	}

	actionFile := filepath.Join(linkRoot, "bind.go")
	got, err := moduleCopyHelperDependencyFiles(linkRoot, []string{actionFile})
	if err != nil {
		t.Fatalf("moduleCopyHelperDependencyFiles() error = %v", err)
	}

	var found bool
	for _, file := range got {
		if filepath.Base(file) == "binding_challenge.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("moduleCopyHelperDependencyFiles() = %v, want binding_challenge.go", got)
	}
}

func TestModuleCopyHelperDependencyFilesFindsServiceHelpersFromAllActions(t *testing.T) {
	sourceServiceDir := writeCopyTestServiceDependencyFiles(t)
	actionFiles := []string{
		filepath.Join(sourceServiceDir, "bind.go"),
		filepath.Join(sourceServiceDir, "check.go"),
		filepath.Join(sourceServiceDir, "confirm.go"),
	}

	got, err := moduleCopyHelperDependencyFiles(sourceServiceDir, actionFiles)
	if err != nil {
		t.Fatalf("moduleCopyHelperDependencyFiles() error = %v", err)
	}

	want := map[string]bool{
		"backup_code.go":       false,
		"binding_challenge.go": false,
		"verification_code.go": false,
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
func Service(...bool) {}
func Filename(string) {}
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

func (CopyTest) Design() {
	dsl.Route("copytest", func() {
		dsl.Create(func() {
			dsl.Service()
			dsl.Filename("bind.go")
		})
		dsl.List(func() {
			dsl.Service()
			dsl.Filename("check.go")
		})
		dsl.Get(func() {
			dsl.Service()
			dsl.Filename("confirm.go")
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
	write("bind.go", `package servicecopytest

import "github.com/hydroan/gst/service"

type Binder struct {
	service.Base[any, any, any]
}

func (b *Binder) Create() string {
	return bindingChallenge() + verificationCode()
}
`)
	write("check.go", `package servicecopytest

import "github.com/hydroan/gst/service"

type Checker struct {
	service.Base[any, any, any]
}

func (c *Checker) List() string {
	return backupCode()
}
`)
	write("confirm.go", `package servicecopytest

import "github.com/hydroan/gst/service"

type Confirmer struct {
	service.Base[any, any, any]
}

func (c *Confirmer) Get() string {
	return verificationCode()
}
`)
	write("binding_challenge.go", `package servicecopytest

func bindingChallenge() string {
	return "challenge"
}
`)
	write("backup_code.go", `package servicecopytest

func backupCode() string {
	return "backup"
}
`)
	write("verification_code.go", `package servicecopytest

func verificationCode() string {
	return "code"
}
	`)
}

func writeNestedCopyTestModuleSource(t *testing.T, projectDir string) {
	t.Helper()
	frameworkRoot := filepath.Join(projectDir, "internal", "gst")
	if err := os.WriteFile(filepath.Join(frameworkRoot, "module", "copytest", moduleManifestFilename), []byte(`{"copy":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	sourceModelDir := filepath.Join(frameworkRoot, "internal", "model", "copytest", "account")
	if err := os.MkdirAll(sourceModelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceModelDir, "account.go"), []byte(`package modelcopytestaccount

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Account struct {
	model.Empty
}

func (Account) Design() {
	dsl.Route("copytest/account", func() {
		dsl.Create(func() {
			dsl.Service()
			dsl.Filename("create.go")
		})
	})
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	sourceAccountServiceDir := filepath.Join(frameworkRoot, "internal", "service", "copytest", "account")
	if err := os.MkdirAll(sourceAccountServiceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceAccountServiceDir, "create.go"), []byte(`package servicecopytestaccount

import (
	modelcopytestaccount "github.com/hydroan/gst/internal/model/copytest/account"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type AccountCreateService struct {
	service.Base[*modelcopytestaccount.Account, *modelcopytestaccount.Account, *modelcopytestaccount.Account]
}

func (s *AccountCreateService) Create(ctx *types.ServiceContext, req *modelcopytestaccount.Account) (rsp *modelcopytestaccount.Account, err error) {
	return accountHelper(req), nil
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceAccountServiceDir, "helper.go"), []byte(`package servicecopytestaccount

import modelcopytestaccount "github.com/hydroan/gst/internal/model/copytest/account"

func accountHelper(req *modelcopytestaccount.Account) *modelcopytestaccount.Account {
	return req
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceAccountServiceDir, "standalone.go"), []byte(`package servicecopytestaccount

func standaloneAccountHelper() string {
	return "standalone"
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	sourceAuditServiceDir := filepath.Join(frameworkRoot, "internal", "service", "copytest", "audit")
	if err := os.MkdirAll(sourceAuditServiceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceAuditServiceDir, "audit.go"), []byte(`package servicecopytestaudit

func Audit() string {
	return "audit"
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
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

func TestBuildModuleCopyPlanIncludesServiceHelperFiles(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeCopyTestModuleSource(t, projectDir, nil)
	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("buildModuleCopyPlan() error = %v", err)
	}

	helpers := plan.HelperTargets()
	want := map[string]bool{
		filepath.Join("service", "copytest", "backup_code.go"):       false,
		filepath.Join("service", "copytest", "binding_challenge.go"): false,
		filepath.Join("service", "copytest", "verification_code.go"): false,
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

func TestBuildModuleCopyPlanCopiesNestedActionsAndReachableHelpers(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeNestedCopyTestModuleSource(t, projectDir)
	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	modelTargets := plan.ModelTargets()
	if !slices.Contains(modelTargets, filepath.Join("model", "copytest", "account", "account.go")) {
		t.Fatalf("ModelTargets() = %v, want nested account model", modelTargets)
	}

	serviceTargets := plan.ServiceTargets()
	if !slices.Contains(serviceTargets, filepath.Join("service", "copytest", "account", "create.go")) {
		t.Fatalf("ServiceTargets() = %v, want nested account action service", serviceTargets)
	}

	helperTargets := plan.HelperTargets()
	reachableHelper := filepath.Join("service", "copytest", "account", "helper.go")
	if !slices.Contains(helperTargets, reachableHelper) {
		t.Fatalf("HelperTargets() = %v, want %s", helperTargets, reachableHelper)
	}
	for _, unwanted := range []string{
		filepath.Join("service", "copytest", "account", "standalone.go"),
		filepath.Join("service", "copytest", "audit", "audit.go"),
	} {
		if slices.Contains(helperTargets, unwanted) {
			t.Fatalf("HelperTargets() = %v, should not include unrelated service file %s", helperTargets, unwanted)
		}
	}

	accountModel := moduleCopyPlanFileContent(t, plan, filepath.Join("model", "copytest", "account", "account.go"))
	if !strings.HasPrefix(accountModel, "package account\n") {
		t.Fatalf("nested model package mismatch:\n%s", accountModel)
	}
	accountService := moduleCopyPlanFileContent(t, plan, filepath.Join("service", "copytest", "account", "create.go"))
	for _, want := range []string{
		"package account\n",
		`"tmpapp/model/copytest/account"`,
		"func (c *Create) Create(ctx *types.ServiceContext, req *account.Account) (rsp *account.Account, err error)",
	} {
		if !strings.Contains(accountService, want) {
			t.Fatalf("nested action service missing %q:\n%s", want, accountService)
		}
	}
	if strings.Contains(accountService, "modelcopytestaccount") {
		t.Fatalf("nested action service leaked source package alias:\n%s", accountService)
	}

	accountHelper := moduleCopyPlanFileContent(t, plan, filepath.Join("service", "copytest", "account", "helper.go"))
	if !strings.Contains(accountHelper, "package account\n") || !strings.Contains(accountHelper, `"tmpapp/model/copytest/account"`) || strings.Contains(accountHelper, "modelcopytestaccount") {
		t.Fatalf("nested helper was not normalized:\n%s", accountHelper)
	}
}

func TestBuildModuleCopyPlanIncludesMiddlewareFiles(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	frameworkRoot := filepath.Join(projectDir, "internal", "gst")
	manifest := []byte(`{
		"copy": {
			"middleware": [
				{"sourceFile": "middleware/copy_auth.go", "scope": "auth", "handler": "CopyAuth"}
			]
		}
	}`)
	writeCopyTestModuleSource(t, projectDir, manifest)
	if err := os.MkdirAll(filepath.Join(frameworkRoot, "middleware"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "middleware", "copy_auth.go"), []byte(`package middleware

func CopyAuth() any {
	return nil
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	targets := plan.MiddlewareTargets()
	if !slices.Contains(targets, filepath.Join("middleware", "copy_auth.go")) {
		t.Fatalf("MiddlewareTargets() = %v, want middleware/copy_auth.go", targets)
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

func CopyAuth() any {
	return nil
}
`)
	plan := &CopyPlan{
		Name:                "copytest",
		ModelDir:            "model",
		ServiceDir:          "service",
		TargetMiddlewareDir: "middleware",
		Files: []moduleCopyFile{
			{
				Kind:       moduleCopyFileMiddleware,
				TargetPath: filepath.Join("middleware", "copy_auth.go"),
				Content:    source,
			},
		},
		Middleware: []moduleCopyMiddleware{
			{
				SourcePath: filepath.Join("internal", "gst", "middleware", "copy_auth.go"),
				TargetPath: filepath.Join("middleware", "copy_auth.go"),
				Scope:      moduleCopyMiddlewareScopeAuth,
				Handler:    "CopyAuth",
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

	copied, err := os.ReadFile(filepath.Join(projectDir, "middleware", "copy_auth.go"))
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
	if !strings.Contains(code, "middleware.RegisterAuth(CopyAuth())") {
		t.Fatalf("auth middleware registration missing:\n%s", code)
	}
	if strings.Contains(code, "gstmiddleware") {
		t.Fatalf("middleware registration used an unnecessary alias:\n%s", code)
	}
}

func TestBuildModuleCopyPlanReportsExtraTargetModelFiles(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeCopyTestModuleSource(t, projectDir, nil)

	targetModelDir := filepath.Join(projectDir, "model", "copytest")
	if mkdirErr := os.MkdirAll(targetModelDir, 0o755); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	extraTarget := filepath.Join(targetModelDir, "design.go")
	if writeErr := os.WriteFile(extraTarget, []byte("package copytest\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(targetModelDir, "design_test.go"), []byte("package copytest\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	extraTargets := plan.ExtraModelTargets()
	if len(extraTargets) != 1 {
		t.Fatalf("ExtraModelTargets() = %v, want one extra target", extraTargets)
	}
	want := filepath.Join("model", "copytest", "design.go")
	if extraTargets[0] != want {
		t.Fatalf("ExtraModelTargets()[0] = %q, want %q", extraTargets[0], want)
	}
}

func TestBuildModuleCopyPlanReportsExtraTargetServiceFiles(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeCopyTestModuleSource(t, projectDir, nil)

	targetServiceDir := filepath.Join(projectDir, "service", "copytest")
	if mkdirErr := os.MkdirAll(targetServiceDir, 0o755); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	extraTarget := filepath.Join(targetServiceDir, "project_adapter.go")
	if writeErr := os.WriteFile(extraTarget, []byte("package copytest\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(targetServiceDir, "project_adapter_test.go"), []byte("package copytest\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	extraTargets := plan.ExtraServiceTargets()
	if len(extraTargets) != 1 {
		t.Fatalf("ExtraServiceTargets() = %v, want one extra target", extraTargets)
	}
	want := filepath.Join("service", "copytest", "project_adapter.go")
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
		dsl.Service()
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
