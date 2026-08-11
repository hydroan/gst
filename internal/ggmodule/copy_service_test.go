package ggmodule

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

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
	"tmpapp/model/copytest"

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
		SourcePath: "action.go",
		Source:     source,
		TargetPath: "service/copytest/action.go",
		Target:     target,
		Rewrite: moduleCopyRewriteConfig{
			ModuleName:        "copytest",
			ProjectModulePath: "tmpapp",
			ModelDir:          "model",
			ServiceDir:        "service",
			TargetPackage:     "copytest",
		},
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

// TestMergeModuleServiceSourceDropsImportOnlyUsedByReplacedStruct covers the
// import that the target service shell strands. The shell owns the service
// struct, so a source import used only by the replaced struct declaration has
// no reference left in the merged file and must not reach the copied project.
func TestMergeModuleServiceSourceDropsImportOnlyUsedByReplacedStruct(t *testing.T) {
	source := []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ActionService runs the source action flow.
type ActionService struct {
	service.Base[*model.Empty, *modelcopytest.ActionReq, *modelcopytest.ActionRsp]
}

// Create copies the source business logic.
func (s *ActionService) Create(ctx *types.ServiceContext, req *modelcopytest.ActionReq) (rsp *modelcopytest.ActionRsp, err error) {
	return &modelcopytest.ActionRsp{}, nil
}
`)
	target := []byte(`package copytest

import (
	"tmpapp/model/copytest"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*copytest.Action, *copytest.ActionReq, *copytest.ActionRsp]
}

func (c *Creator) Create(ctx *types.ServiceContext, req *copytest.ActionReq) (rsp *copytest.ActionRsp, err error) {
	return rsp, nil
}
`)

	got, err := mergeModuleServiceSource(moduleServiceMergeInput{
		SourcePath: "action.go",
		Source:     source,
		TargetPath: "service/copytest/action.go",
		Target:     target,
		Rewrite: moduleCopyRewriteConfig{
			ModuleName:        "copytest",
			ProjectModulePath: "tmpapp",
			ModelDir:          "model",
			ServiceDir:        "service",
			TargetPackage:     "copytest",
		},
	})
	if err != nil {
		t.Fatalf("mergeModuleServiceSource() error = %v", err)
	}

	code := string(got)
	if strings.Contains(code, `"github.com/hydroan/gst/model"`) {
		t.Fatalf("import stranded by the replaced service struct was kept:\n%s", code)
	}
	if !strings.Contains(code, "service.Base[*copytest.Action, *copytest.ActionReq, *copytest.ActionRsp]") {
		t.Fatalf("target service struct was not preserved:\n%s", code)
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
	"tmpapp/model/copytest"

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
		SourcePath: "list.go",
		Source:     source,
		TargetPath: "service/copytest/list.go",
		Target:     target,
		Rewrite: moduleCopyRewriteConfig{
			ModuleName:        "copytest",
			ProjectModulePath: "tmpapp",
			ModelDir:          "model",
			ServiceDir:        "service",
			TargetPackage:     "copytest",
		},
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
	"tmpapp/model/copytest"

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
		SourcePath: "binding.go",
		Source:     source,
		TargetPath: "service/copytest/binding.go",
		Target:     target,
		Rewrite: moduleCopyRewriteConfig{
			ModuleName:        "copytest",
			ProjectModulePath: "tmpapp",
			ModelDir:          "model",
			ServiceDir:        "service",
			TargetPackage:     "copytest",
		},
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

func TestMergeModuleServiceSourceRejectsDuplicateMethodNames(t *testing.T) {
	source := []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type FirstService struct {
	service.Base[*modelcopytest.CopyTest, *modelcopytest.CopyTest, *modelcopytest.CopyTest]
}

type SecondService struct {
	service.Base[*modelcopytest.CopyTest, *modelcopytest.CopyTest, *modelcopytest.CopyTest]
}

func (f *FirstService) CreateBefore(ctx *types.ServiceContext, req *modelcopytest.CopyTest) error {
	return nil
}

func (s *SecondService) CreateBefore(ctx *types.ServiceContext, req *modelcopytest.CopyTest) error {
	return nil
}
`)
	target := []byte(`package copytest

import (
	"tmpapp/model/copytest"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*copytest.CopyTest, *copytest.CopyTest, *copytest.CopyTest]
}

func (c *Creator) Create(ctx *types.ServiceContext, req *copytest.CopyTest) (rsp *copytest.CopyTest, err error) {
	return rsp, nil
}
`)

	_, err := mergeModuleServiceSource(moduleServiceMergeInput{
		SourcePath: "create.go",
		Source:     source,
		TargetPath: "service/copytest/create.go",
		Target:     target,
		Rewrite: moduleCopyRewriteConfig{
			ModuleName:        "copytest",
			ProjectModulePath: "tmpapp",
			ModelDir:          "model",
			ServiceDir:        "service",
			TargetPackage:     "copytest",
		},
	})
	if err == nil {
		t.Fatal("mergeModuleServiceSource() succeeded, want an error for a method declared on two source service structs")
	}
	if !strings.Contains(err.Error(), "CreateBefore") {
		t.Fatalf("mergeModuleServiceSource() error = %v, want the conflicting method name", err)
	}
}

func TestMergeModuleServiceSourceKeepsNonServiceReceiverDoc(t *testing.T) {
	source := []byte(`package servicecopytest

import (
	modelcopytest "github.com/hydroan/gst/internal/model/copytest"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type ActionService struct {
	service.Base[*modelcopytest.CopyTest, *modelcopytest.CopyTest, *modelcopytest.CopyTest]
}

func (s *ActionService) CreateAfter(ctx *types.ServiceContext, req *modelcopytest.CopyTest) error {
	return nil
}

type recordState struct{}

// describe copies the doc of a method on a non-service receiver.
func (r *recordState) describe() string {
	return "described"
}
`)
	target := []byte(`package copytest

import (
	"tmpapp/model/copytest"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*copytest.CopyTest, *copytest.CopyTest, *copytest.CopyTest]
}

func (c *Creator) Create(ctx *types.ServiceContext, req *copytest.CopyTest) (rsp *copytest.CopyTest, err error) {
	return rsp, nil
}
`)

	got, err := mergeModuleServiceSource(moduleServiceMergeInput{
		SourcePath: "create.go",
		Source:     source,
		TargetPath: "service/copytest/create.go",
		Target:     target,
		Rewrite: moduleCopyRewriteConfig{
			ModuleName:        "copytest",
			ProjectModulePath: "tmpapp",
			ModelDir:          "model",
			ServiceDir:        "service",
			TargetPackage:     "copytest",
		},
	})
	if err != nil {
		t.Fatalf("mergeModuleServiceSource() error = %v", err)
	}
	code := string(got)

	if !strings.Contains(code, "func (r *recordState) describe() string") {
		t.Fatalf("non-service receiver method was not copied:\n%s", code)
	}
	if !strings.Contains(code, "// describe copies the doc of a method on a non-service receiver.\nfunc (r *recordState) describe() string") {
		t.Fatalf("non-service receiver method doc was dropped:\n%s", code)
	}
}

// TestDropUnusedMergedServiceImportsRemovesStrandedQualifier pins the narrow
// rule: an import is dropped only when the source proved its local name is the
// real package name and the merged file no longer qualifies anything with it.
func TestDropUnusedMergedServiceImportsRemovesStrandedQualifier(t *testing.T) {
	source := parseModuleCopyTestFile(t, `package servicecopytest

import "github.com/hydroan/gst/model"

type ActionService struct {
	service.Base[*model.Empty, *model.Empty, *model.Empty]
}
`)
	merged := parseModuleCopyTestFile(t, `package copytest

import (
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
)

func (c *Creator) Create(ctx *types.ServiceContext) error {
	return nil
}
`)

	dropUnusedMergedServiceImports(merged, moduleCopyPackageQualifiers(source))

	if importPaths := moduleCopyTestImportPaths(merged); strings.Join(importPaths, ",") != "github.com/hydroan/gst/types" {
		t.Fatalf("dropUnusedMergedServiceImports() imports = %v, want only types", importPaths)
	}
}

// TestDropUnusedMergedServiceImportsKeepsImportWhosePathBaseIsNotPackageName
// guards the drop rule against paths whose last segment is not the package
// name, such as github.com/skip2/go-qrcode declaring package qrcode. The
// source never qualifies anything with "go-qrcode", so the merged file must
// keep the import instead of guessing it is unused.
func TestDropUnusedMergedServiceImportsKeepsImportWhosePathBaseIsNotPackageName(t *testing.T) {
	source := parseModuleCopyTestFile(t, `package servicecopytest

import "github.com/skip2/go-qrcode"

func encode() ([]byte, error) {
	return qrcode.Encode("payload", qrcode.Medium, 256)
}
`)
	merged := parseModuleCopyTestFile(t, `package copytest

import "github.com/skip2/go-qrcode"

func encode() ([]byte, error) {
	return qrcode.Encode("payload", qrcode.Medium, 256)
}
`)

	dropUnusedMergedServiceImports(merged, moduleCopyPackageQualifiers(source))

	if importPaths := moduleCopyTestImportPaths(merged); strings.Join(importPaths, ",") != "github.com/skip2/go-qrcode" {
		t.Fatalf("dropUnusedMergedServiceImports() imports = %v, want the qrcode import kept", importPaths)
	}
}

// TestRequireUsedModuleCopyImportsRejectsUnusedFrameworkImport is the guard that
// keeps a future rewrite from shipping a service file that cannot compile.
func TestRequireUsedModuleCopyImportsRejectsUnusedFrameworkImport(t *testing.T) {
	config := moduleCopyRewriteConfig{ProjectModulePath: "tmpapp"}

	t.Run("unused framework import", func(t *testing.T) {
		file := parseModuleCopyTestFile(t, `package copytest

import (
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
)

func (c *Creator) Create(ctx *types.ServiceContext) error {
	return nil
}
`)

		err := requireUsedModuleCopyImports(file, "service/copytest/action.go", config)
		if err == nil {
			t.Fatal("requireUsedModuleCopyImports() error = nil, want an unused import error")
		}
		if !strings.Contains(err.Error(), "github.com/hydroan/gst/model") {
			t.Fatalf("requireUsedModuleCopyImports() error = %v, want it to name the unused import", err)
		}
	})

	t.Run("unused project import", func(t *testing.T) {
		file := parseModuleCopyTestFile(t, `package copytest

import (
	"tmpapp/model/copytest"
	"github.com/hydroan/gst/types"
)

func (c *Creator) Create(ctx *types.ServiceContext) error {
	return nil
}
`)

		err := requireUsedModuleCopyImports(file, "service/copytest/action.go", config)
		if err == nil {
			t.Fatal("requireUsedModuleCopyImports() error = nil, want an unused import error")
		}
	})

	t.Run("third party import is not checked", func(t *testing.T) {
		file := parseModuleCopyTestFile(t, `package copytest

import (
	"github.com/skip2/go-qrcode"
	"github.com/hydroan/gst/types"
)

func (c *Creator) Create(ctx *types.ServiceContext) error {
	return qrcode.Check()
}
`)

		if err := requireUsedModuleCopyImports(file, "service/copytest/action.go", config); err != nil {
			t.Fatalf("requireUsedModuleCopyImports() error = %v, want third party imports to be skipped", err)
		}
	})

	t.Run("used imports pass", func(t *testing.T) {
		file := parseModuleCopyTestFile(t, `package copytest

import (
	"tmpapp/model/copytest"

	"github.com/hydroan/gst/types"
)

func (c *Creator) Create(ctx *types.ServiceContext) (*copytest.ActionRsp, error) {
	return nil, nil
}
`)

		if err := requireUsedModuleCopyImports(file, "service/copytest/action.go", config); err != nil {
			t.Fatalf("requireUsedModuleCopyImports() error = %v, want no error", err)
		}
	})
}

func parseModuleCopyTestFile(t *testing.T, src string) *ast.File {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parser.ParseFile() error = %v", err)
	}
	return file
}

func moduleCopyTestImportPaths(file *ast.File) []string {
	paths := make([]string, 0, len(file.Imports))
	for _, imp := range file.Imports {
		paths = append(paths, strings.Trim(imp.Path.Value, `"`))
	}
	return paths
}
