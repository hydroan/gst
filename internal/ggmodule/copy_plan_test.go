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

func TestValidateModuleCommandNameRejectsPaths(t *testing.T) {
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
			if err := validateModuleCommandName(name, "module copy"); err == nil {
				t.Fatalf("validateModuleCommandName(%q) succeeded, want error", name)
			}
		})
	}

	if err := validateModuleCommandName("copytest", "module copy"); err != nil {
		t.Fatalf("validateModuleCommandName(%q) = %v, want nil", "copytest", err)
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
		"sample.go": `package servicecopytest

import "github.com/hydroan/gst/service"

type SampleService struct {
	service.Base[any, any, any]
}
`,
		"record.go": `package servicecopytest

import "github.com/hydroan/gst/service"

type RecordService struct {
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
			ModelFilePath: filepath.Join("model", "copytest", "sample.go"),
			ModelPkgName:  "copytest",
			ModelName:     "Sample",
			ModelVarName:  "s",
			Design: &dsl.Design{
				Enabled:    true,
				Endpoint:   "samples",
				Create:     &dsl.Action{Enabled: true, Service: true, Filename: "sample.go", Phase: consts.PHASE_CREATE},
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
			ModelFilePath: filepath.Join("model", "copytest", "record.go"),
			ModelPkgName:  "copytest",
			ModelName:     "Record",
			ModelVarName:  "r",
			Design: &dsl.Design{
				Enabled:    true,
				Endpoint:   "records",
				Create:     &dsl.Action{},
				Delete:     &dsl.Action{},
				Update:     &dsl.Action{},
				Patch:      &dsl.Action{},
				List:       &dsl.Action{Enabled: true, Service: true, Filename: "record.go", Phase: consts.PHASE_LIST},
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
	if gotModelsByFile["sample.go"] != "Sample" {
		t.Fatalf("sample.go action model = %q, want Sample", gotModelsByFile["sample.go"])
	}
	if gotModelsByFile["record.go"] != "Record" {
		t.Fatalf("record.go action model = %q, want Record", gotModelsByFile["record.go"])
	}
}

func TestAddServiceFilesMergesActionsSharingServiceFile(t *testing.T) {
	sourceServiceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceServiceDir, "sample.go"), []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// SampleService owns sample hooks.
type SampleService struct {
	service.Base[*modelcopytest.CopyTest, *modelcopytest.CopyTest, *modelcopytest.CopyTest]
}

// CreateAfter copies create hook logic.
func (s *SampleService) CreateAfter(ctx *types.ServiceContext, req *modelcopytest.CopyTest) error {
	return nil
}

// DeleteAfter copies delete hook logic.
func (s *SampleService) DeleteAfter(ctx *types.ServiceContext, req *modelcopytest.CopyTest) error {
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
		Filename: "sample.go",
		Payload:  "*CopyTest",
		Result:   "*CopyTest",
		Phase:    consts.PHASE_CREATE,
	}
	deleteAction := &dsl.Action{
		Enabled:  true,
		Service:  true,
		Filename: "sample.go",
		Payload:  "*CopyTest",
		Result:   "*CopyTest",
		Phase:    consts.PHASE_DELETE,
	}
	plan := &CopyPlan{
		Name:                  "copytest",
		ProjectModulePath:     "tmpapp",
		ModelDir:              "model",
		ServiceDir:            "service",
		SourceServiceDir:      sourceServiceDir,
		TargetServiceDir:      filepath.Join("service", "copytest"),
		TargetModelImportPath: filepath.Join("tmpapp", "model", "copytest"),
		Actions: []moduleCopyAction{
			{
				Action:     createAction,
				SourcePath: filepath.Join(sourceServiceDir, "sample.go"),
				TargetPath: filepath.Join("service", "copytest", "sample.go"),
				ModelInfo:  modelInfo,
			},
			{
				Action:     deleteAction,
				SourcePath: filepath.Join(sourceServiceDir, "sample.go"),
				TargetPath: filepath.Join("service", "copytest", "sample.go"),
				ModelInfo:  modelInfo,
			},
		},
	}

	if err := plan.addServiceFiles(nil); err != nil {
		t.Fatalf("addServiceFiles() error = %v", err)
	}
	targets := plan.ServiceTargets()
	if len(targets) != 1 {
		t.Fatalf("ServiceTargets() = %v, want one merged sample.go target", targets)
	}
	code := string(plan.Files[0].Content)
	for _, want := range []string{
		"func (s *Sample) Create(ctx *types.ServiceContext, req *copytest.CopyTest) (rsp *copytest.CopyTest, err error)",
		"func (s *Sample) Delete(ctx *types.ServiceContext, req *copytest.CopyTest) (rsp *copytest.CopyTest, err error)",
		"// CreateAfter copies create hook logic.\nfunc (s *Sample) CreateAfter",
		"// DeleteAfter copies delete hook logic.\nfunc (s *Sample) DeleteAfter",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("merged service file missing %q:\n%s", want, code)
		}
	}
}

func TestAddServiceFilesMergesActionsFromMultipleSourceServiceStructs(t *testing.T) {
	sourceServiceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceServiceDir, "item.go"), []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ItemGetService handles reads.
type ItemGetService struct {
	service.Base[*modelcopytest.Item, *modelcopytest.ItemGetReq, *modelcopytest.ItemGetRsp]
}

// ItemPatchService handles writes.
type ItemPatchService struct {
	service.Base[*modelcopytest.Item, *modelcopytest.ItemPatchReq, *modelcopytest.ItemPatchRsp]
}

// Get copies get logic.
func (s *ItemGetService) Get(ctx *types.ServiceContext, req *modelcopytest.ItemGetReq) (rsp *modelcopytest.ItemGetRsp, err error) {
	return itemGetResult(), nil
}

// Patch copies patch logic.
func (s *ItemPatchService) Patch(ctx *types.ServiceContext, req *modelcopytest.ItemPatchReq) (rsp *modelcopytest.ItemPatchRsp, err error) {
	return itemPatchResult(), nil
}

func itemGetResult() *modelcopytest.Item {
	return nil
}

func itemPatchResult() *modelcopytest.Item {
	return nil
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	modelInfo := &gen.ModelInfo{
		ModulePath:    "tmpapp",
		ModelFileDir:  filepath.Join("model", "copytest"),
		ModelFilePath: filepath.Join("model", "copytest", "item.go"),
		ModelPkgName:  "copytest",
		ModelName:     "Item",
		ModelVarName:  "i",
		Design:        &dsl.Design{Enabled: true},
	}
	getAction := &dsl.Action{
		Enabled:  true,
		Service:  true,
		Filename: "item.go",
		Flatten:  true,
		Payload:  "*ItemGetReq",
		Result:   "*ItemGetRsp",
		Phase:    consts.PHASE_GET,
	}
	patchAction := &dsl.Action{
		Enabled:  true,
		Service:  true,
		Filename: "item.go",
		Flatten:  true,
		Payload:  "*ItemPatchReq",
		Result:   "*ItemPatchRsp",
		Phase:    consts.PHASE_PATCH,
	}
	plan := &CopyPlan{
		Name:                  "copytest",
		ProjectModulePath:     "tmpapp",
		ModelDir:              "model",
		ServiceDir:            "service",
		SourceServiceDir:      sourceServiceDir,
		TargetServiceDir:      filepath.Join("service", "copytest"),
		TargetModelImportPath: filepath.Join("tmpapp", "model", "copytest"),
		Actions: []moduleCopyAction{
			{
				Action:     getAction,
				SourcePath: filepath.Join(sourceServiceDir, "item.go"),
				TargetPath: filepath.Join("service", "copytest", "item.go"),
				ModelInfo:  modelInfo,
			},
			{
				Action:     patchAction,
				SourcePath: filepath.Join(sourceServiceDir, "item.go"),
				TargetPath: filepath.Join("service", "copytest", "item.go"),
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
		"// Get copies get logic.\nfunc (i *Item) Get",
		"return itemGetResult(), nil",
		"// Patch copies patch logic.\nfunc (i *Item) Patch",
		"return itemPatchResult(), nil",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("merged service file missing %q:\n%s", want, code)
		}
	}
	for _, unwanted := range []string{
		"type ItemGetService struct",
		"type ItemPatchService struct",
		"func (s *ItemGetService)",
		"func (s *ItemPatchService)",
	} {
		if strings.Contains(code, unwanted) {
			t.Fatalf("merged service file kept source service artifact %q:\n%s", unwanted, code)
		}
	}
}

func TestAddServiceFilesKeepsEmptyPayloadListRequest(t *testing.T) {
	sourceServiceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceServiceDir, "list.go"), []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ItemListService lists items.
type ItemListService struct {
	service.Base[*modelcopytest.Item, *model.Empty, *modelcopytest.ItemListRsp]
}

// List copies list logic.
func (s *ItemListService) List(ctx *types.ServiceContext, req *model.Empty) (rsp *modelcopytest.ItemListRsp, err error) {
	return &modelcopytest.ItemListRsp{}, nil
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	modelInfo := &gen.ModelInfo{
		ModulePath:    "tmpapp",
		ModelFileDir:  filepath.Join("model", "copytest"),
		ModelFilePath: filepath.Join("model", "copytest", "item.go"),
		ModelPkgName:  "copytest",
		ModelName:     "Item",
		ModelVarName:  "i",
		Design:        &dsl.Design{Enabled: true},
	}
	listAction := &dsl.Action{
		Enabled: true,
		Service: true,
		Payload: dsl.PayloadEmpty,
		Result:  "*ItemListRsp",
		Phase:   consts.PHASE_LIST,
	}
	plan := &CopyPlan{
		Name:                  "copytest",
		ProjectModulePath:     "tmpapp",
		ModelDir:              "model",
		ServiceDir:            "service",
		SourceServiceDir:      sourceServiceDir,
		TargetServiceDir:      filepath.Join("service", "copytest"),
		TargetModelImportPath: filepath.Join("tmpapp", "model", "copytest"),
		Actions: []moduleCopyAction{
			{
				Action:     listAction,
				SourcePath: filepath.Join(sourceServiceDir, "list.go"),
				TargetPath: filepath.Join("service", "copytest", "list.go"),
				ModelInfo:  modelInfo,
			},
		},
	}

	if err := plan.addServiceFiles(nil); err != nil {
		t.Fatalf("addServiceFiles() error = %v", err)
	}
	code := string(plan.Files[0].Content)
	// The copy pipeline rewrites the module model import to the target
	// project, but the gst model import backing *model.Empty must survive
	// untouched.
	for _, want := range []string{
		`"github.com/hydroan/gst/model"`,
		"service.Base[*copytest.Item, *model.Empty, *copytest.ItemListRsp]",
		"req *model.Empty",
		"return &copytest.ItemListRsp{}, nil",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("copied empty-payload list service missing %q:\n%s", want, code)
		}
	}
	if strings.Contains(code, "modelcopytest") {
		t.Fatalf("copied service kept source module import artifacts:\n%s", code)
	}
}

func TestAddServiceFilesUsesFlattenServicePackage(t *testing.T) {
	sourceServiceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceServiceDir, "sample.go"), []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type SampleService struct {
	service.Base[*modelcopytest.Sample, *modelcopytest.Sample, *modelcopytest.Sample]
}

func (s *SampleService) CreateAfter(ctx *types.ServiceContext, req *modelcopytest.Sample) error {
	return nil
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	modelInfo := &gen.ModelInfo{
		ModulePath:    "tmpapp",
		ModelFileDir:  filepath.Join("model", "copytest"),
		ModelFilePath: filepath.Join("model", "copytest", "sample.go"),
		ModelPkgName:  "copytest",
		ModelName:     "Sample",
		ModelVarName:  "s",
		Design:        &dsl.Design{Enabled: true},
	}
	createAction := &dsl.Action{
		Enabled:  true,
		Service:  true,
		Filename: "sample.go",
		Flatten:  true,
		Payload:  "*Sample",
		Result:   "*Sample",
		Phase:    consts.PHASE_CREATE,
	}
	plan := &CopyPlan{
		Name:                  "copytest",
		ProjectModulePath:     "tmpapp",
		ModelDir:              "model",
		ServiceDir:            "service",
		SourceServiceDir:      sourceServiceDir,
		TargetServiceDir:      filepath.Join("service", "copytest"),
		TargetModelImportPath: filepath.Join("tmpapp", "model", "copytest"),
		Actions: []moduleCopyAction{
			{
				Action:     createAction,
				SourcePath: filepath.Join(sourceServiceDir, "sample.go"),
				TargetPath: filepath.Join("service", "copytest", "sample.go"),
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
	if strings.HasPrefix(code, "package sample\n") {
		t.Fatalf("flattened merged service kept sample package:\n%s", code)
	}
}

func TestBuildCopyPlanIncludesServiceHelperFiles(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeCopyTestModuleSource(t, projectDir, nil)
	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	helpers := plan.HelperTargets()
	want := map[string]bool{
		filepath.Join("service", "copytest", "list_helper.go"):   false,
		filepath.Join("service", "copytest", "create_helper.go"): false,
		filepath.Join("service", "copytest", "shared_helper.go"): false,
	}
	for _, helper := range helpers {
		if _, ok := want[helper]; ok {
			want[helper] = true
		}
	}
	for helper, found := range want {
		if !found {
			t.Fatalf("HelperTargets() = %v, want %s", helpers, helper)
		}
	}
}

func TestBuildCopyPlanCopiesNestedActionsAndReachableHelpers(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeNestedCopyTestModuleSource(t, projectDir)
	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	modelTargets := plan.ModelTargets()
	if !slices.Contains(modelTargets, filepath.Join("model", "copytest", "entry", "entry.go")) {
		t.Fatalf("ModelTargets() = %v, want nested entry model", modelTargets)
	}

	serviceTargets := plan.ServiceTargets()
	if !slices.Contains(serviceTargets, filepath.Join("service", "copytest", "entry", "create.go")) {
		t.Fatalf("ServiceTargets() = %v, want nested entry action service", serviceTargets)
	}

	helperTargets := plan.HelperTargets()
	reachableHelper := filepath.Join("service", "copytest", "entry", "helper.go")
	if !slices.Contains(helperTargets, reachableHelper) {
		t.Fatalf("HelperTargets() = %v, want %s", helperTargets, reachableHelper)
	}
	importedHelper := filepath.Join("service", "copytest", "shared", "shared.go")
	if !slices.Contains(helperTargets, importedHelper) {
		t.Fatalf("HelperTargets() = %v, want %s", helperTargets, importedHelper)
	}
	for _, unwanted := range []string{
		filepath.Join("service", "copytest", "entry", "standalone.go"),
		filepath.Join("service", "copytest", "unrelated", "unrelated.go"),
	} {
		if slices.Contains(helperTargets, unwanted) {
			t.Fatalf("HelperTargets() = %v, should not include unrelated service file %s", helperTargets, unwanted)
		}
	}

	entryModel := moduleCopyPlanFileContent(t, plan, filepath.Join("model", "copytest", "entry", "entry.go"))
	if !strings.HasPrefix(entryModel, "package entry\n") {
		t.Fatalf("nested model package mismatch:\n%s", entryModel)
	}
	entryService := moduleCopyPlanFileContent(t, plan, filepath.Join("service", "copytest", "entry", "create.go"))
	for _, want := range []string{
		"package entry\n",
		`"tmpapp/model/copytest/entry"`,
		"func (c *Create) Create(ctx *types.ServiceContext, req *entry.Entry) (rsp *entry.Entry, err error)",
	} {
		if !strings.Contains(entryService, want) {
			t.Fatalf("nested action service missing %q:\n%s", want, entryService)
		}
	}
	if strings.Contains(entryService, "modelcopytestentry") {
		t.Fatalf("nested action service leaked source package alias:\n%s", entryService)
	}

	entryHelper := moduleCopyPlanFileContent(t, plan, filepath.Join("service", "copytest", "entry", "helper.go"))
	if !strings.Contains(entryHelper, "package entry\n") || !strings.Contains(entryHelper, `"tmpapp/model/copytest/entry"`) || strings.Contains(entryHelper, "modelcopytestentry") {
		t.Fatalf("nested helper was not normalized:\n%s", entryHelper)
	}

	sharedHelper := moduleCopyPlanFileContent(t, plan, importedHelper)
	if !strings.Contains(sharedHelper, "package shared\n") {
		t.Fatalf("imported service helper was not normalized:\n%s", sharedHelper)
	}
}

func TestBuildCopyPlanReportsExtraTargetModelFiles(t *testing.T) {
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

func TestBuildCopyPlanReportsExtraTargetServiceFiles(t *testing.T) {
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

func TestBuildCopyPlanIgnoresFrameworkRootRelativeFiles(t *testing.T) {
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

func writeNestedCopyTestModuleSource(t *testing.T, projectDir string) {
	t.Helper()
	frameworkRoot := filepath.Join(projectDir, "internal", "gst")
	if err := os.WriteFile(filepath.Join(frameworkRoot, "module", "copytest", moduleManifestFilename), []byte(`{"copy":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	sourceModelDir := filepath.Join(frameworkRoot, "internal", "model", "copytest", "entry")
	if err := os.MkdirAll(sourceModelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceModelDir, "entry.go"), []byte(`package modelcopytestentry

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Entry struct {
	model.Empty
}

func (Entry) Design() {
	dsl.Route("copytest/entry", func() {
		dsl.Create(func() {
			dsl.Service()
			dsl.Filename("create.go")
		})
	})
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	sourceEntryServiceDir := filepath.Join(frameworkRoot, "internal", "service", "copytest", "entry")
	if err := os.MkdirAll(sourceEntryServiceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceEntryServiceDir, "create.go"), []byte(`package servicecopytestentry

import (
	modelcopytestentry "github.com/hydroan/gst/internal/model/copytest/entry"
	"github.com/hydroan/gst/internal/service/copytest/shared"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type EntryCreateService struct {
	service.Base[*modelcopytestentry.Entry, *modelcopytestentry.Entry, *modelcopytestentry.Entry]
}

func (s *EntryCreateService) Create(ctx *types.ServiceContext, req *modelcopytestentry.Entry) (rsp *modelcopytestentry.Entry, err error) {
	shared.Ensure(req)
	return entryHelper(req), nil
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceEntryServiceDir, "helper.go"), []byte(`package servicecopytestentry

import modelcopytestentry "github.com/hydroan/gst/internal/model/copytest/entry"

func entryHelper(req *modelcopytestentry.Entry) *modelcopytestentry.Entry {
	return req
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceEntryServiceDir, "standalone.go"), []byte(`package servicecopytestentry

func standaloneEntryHelper() string {
	return "standalone"
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	sourceUnrelatedServiceDir := filepath.Join(frameworkRoot, "internal", "service", "copytest", "unrelated")
	if err := os.MkdirAll(sourceUnrelatedServiceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceUnrelatedServiceDir, "unrelated.go"), []byte(`package servicecopytestunrelated

func Unrelated() string {
	return "unrelated"
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	sourceSharedServiceDir := filepath.Join(frameworkRoot, "internal", "service", "copytest", "shared")
	if err := os.MkdirAll(sourceSharedServiceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceSharedServiceDir, "shared.go"), []byte(`package shared

func Ensure(any) {}
`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBuildCopyPlanSkipsTestdataAndVendorModelSources(t *testing.T) {
	projectDir := newModuleCopyPlanProject(t)
	writeCopyTestModuleSource(t, projectDir, nil)
	sourceModelDir := filepath.Join(projectDir, "internal", "gst", "internal", "model", "copytest")
	for _, dir := range []string{"testdata", "vendor"} {
		if err := os.MkdirAll(filepath.Join(sourceModelDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sourceModelDir, dir, "ignored.go"), []byte("package ignored\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(projectDir)

	plan, err := BuildCopyPlan("copytest", CopyOptions{})
	if err != nil {
		t.Fatalf("BuildCopyPlan() error = %v", err)
	}

	targets := plan.ModelTargets()
	for _, unwanted := range []string{
		filepath.Join("model", "copytest", "testdata", "ignored.go"),
		filepath.Join("model", "copytest", "vendor", "ignored.go"),
	} {
		if slices.Contains(targets, unwanted) {
			t.Fatalf("ModelTargets() = %v, should not copy %s", targets, unwanted)
		}
	}
}
