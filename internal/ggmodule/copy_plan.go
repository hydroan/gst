package ggmodule

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
)

const frameworkModulePath = "github.com/hydroan/gst"

const (
	defaultModelDir   = "model"
	defaultServiceDir = "service"
)

// CopyOptions configures the local-source copy workflow.
type CopyOptions struct {
	Force      bool
	ModelDir   string
	ServiceDir string
}

func (o CopyOptions) modelDir() string {
	if o.ModelDir == "" {
		return defaultModelDir
	}
	return o.ModelDir
}

func (o CopyOptions) serviceDir() string {
	if o.ServiceDir == "" {
		return defaultServiceDir
	}
	return o.ServiceDir
}

// CopyPlan describes the final files and source mappings for one module copy.
type CopyPlan struct {
	Name                  string
	ProjectModulePath     string
	FrameworkRoot         string
	ModelDir              string
	ServiceDir            string
	SourceModelDir        string
	SourceServiceDir      string
	TargetModelDir        string
	TargetServiceDir      string
	TargetModelImportPath string
	Actions               []moduleCopyAction
	Files                 []moduleCopyFile
	PostCopyNotes         []string
}

// moduleCopyAction connects one DSL action to the framework service file that
// supplies its business logic and the current-project service file that gg gen
// will create for it.
type moduleCopyAction struct {
	Route      string
	Action     *dsl.Action
	SourcePath string
	TargetPath string
	MethodName string
	ModelInfo  *gen.ModelInfo
}

// moduleCopyFile stores final target content. Conflict checks run against this
// final content before any file is written, so pre-existing files only need
// --force when the copy would actually change them.
type moduleCopyFile struct {
	Kind        moduleCopyFileKind
	TargetPath  string
	Content     []byte
	Preexisting bool
}

type moduleCopyFileKind string

const (
	moduleCopyFileModel   moduleCopyFileKind = "model"
	moduleCopyFileService moduleCopyFileKind = "service"
	moduleCopyFileHelper  moduleCopyFileKind = "helper"
)

// BuildCopyPlan is the copy-ready preflight. It resolves framework source
// paths, parses source DSL through the same codegen model parser used by gg gen,
// computes final rewritten file contents, and checks target conflicts.
func BuildCopyPlan(name string, opts CopyOptions) (*CopyPlan, error) {
	if err := validateModuleCopyName(name); err != nil {
		return nil, err
	}
	if _, err := os.Stat("go.mod"); err != nil {
		return nil, fmt.Errorf("gg module copy must run from the project root: %w", err)
	}

	projectModule, err := getModuleName()
	if err != nil {
		return nil, err
	}

	frameworkRoot, err := findFrameworkRoot()
	if err != nil {
		return nil, err
	}

	plan := &CopyPlan{
		Name:                  name,
		ProjectModulePath:     projectModule,
		FrameworkRoot:         frameworkRoot,
		ModelDir:              opts.modelDir(),
		ServiceDir:            opts.serviceDir(),
		SourceModelDir:        filepath.Join(frameworkRoot, "internal", "model", name),
		SourceServiceDir:      filepath.Join(frameworkRoot, "internal", "service", name),
		TargetModelDir:        filepath.Join(opts.modelDir(), name),
		TargetServiceDir:      filepath.Join(opts.serviceDir(), name),
		TargetModelImportPath: filepath.Join(projectModule, opts.modelDir(), name),
	}

	if sourceDirErr := plan.checkSourceDirs(); sourceDirErr != nil {
		return nil, sourceDirErr
	}
	postCopyNotes, err := loadModuleCopyMetadata(filepath.Join(frameworkRoot, "module", name))
	if err != nil {
		return nil, err
	}
	plan.PostCopyNotes = postCopyNotes

	if registerErr := checkModuleNotRegistered(name); registerErr != nil {
		return nil, registerErr
	}

	models, err := codegen.FindModels(frameworkModulePath, plan.SourceModelDir, plan.SourceServiceDir, nil)
	if err != nil {
		return nil, err
	}
	if readyErr := checkModuleCopyReadyModels(plan.SourceModelDir, models); readyErr != nil {
		return nil, readyErr
	}

	if addModelErr := plan.addModelFiles(); addModelErr != nil {
		return nil, addModelErr
	}

	actions, err := plan.collectActions(models)
	if err != nil {
		return nil, err
	}
	plan.Actions = actions

	// Precompute final service/helper contents during preflight so conflict checks
	// compare against what will actually be written. The execution phase still
	// runs gg gen for real before writing these merged files.
	helperFiles, err := moduleCopyHelperDependencyFiles(plan.SourceServiceDir, actionSourcePaths(actions))
	if err != nil {
		return nil, err
	}

	if addServiceErr := plan.addServiceFiles(helperFiles); addServiceErr != nil {
		return nil, addServiceErr
	}
	if conflictErr := plan.checkConflicts(opts.Force); conflictErr != nil {
		return nil, conflictErr
	}
	return plan, nil
}

// validateModuleCopyName intentionally rejects anything path-like. The first
// copy implementation only supports built-in framework modules addressed by
// name, such as "mfa".
func validateModuleCopyName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("module name is required")
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("module name %q must not contain surrounding whitespace", name)
	}
	if strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("module copy accepts a module name, not a path: %s", name)
	}
	if filepath.Clean(name) != name || filepath.Base(name) != name {
		return fmt.Errorf("module copy accepts a module name, not a path: %s", name)
	}
	return nil
}

func findFrameworkRoot() (string, error) {
	candidates := []string{
		filepath.Join(".", "internal", "gst"),
		".",
	}
	for _, candidate := range candidates {
		if isFrameworkRoot(candidate) {
			return filepath.Clean(candidate), nil
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if isFrameworkRoot(wd) {
			return filepath.Clean(wd), nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return "", errors.New("framework source not found; expected internal/gst/go.mod")
}

func isFrameworkRoot(candidate string) bool {
	modFile := filepath.Join(candidate, "go.mod")
	data, err := os.ReadFile(modFile)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "module "+frameworkModulePath)
}

func (p *CopyPlan) checkSourceDirs() error {
	if err := requireDir(filepath.Join(p.FrameworkRoot, "module", p.Name)); err != nil {
		return fmt.Errorf("module %q not found: %w", p.Name, err)
	}
	if err := requireDir(p.SourceModelDir); err != nil {
		return fmt.Errorf("module %q model source not found: %w", p.Name, err)
	}
	if err := requireDir(p.SourceServiceDir); err != nil {
		return fmt.Errorf("module %q service source not found: %w", p.Name, err)
	}
	return nil
}

func requireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

func (p *CopyPlan) addModelFiles() error {
	files, err := goFilesInDir(p.SourceModelDir)
	if err != nil {
		return err
	}
	for _, sourcePath := range files {
		rel, err := filepath.Rel(p.SourceModelDir, sourcePath)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(p.TargetModelDir, rel)
		src, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		content, err := normalizeModuleModelSource(sourcePath, src, p.Name)
		if err != nil {
			return err
		}
		p.Files = append(p.Files, moduleCopyFile{
			Kind:        moduleCopyFileModel,
			TargetPath:  targetPath,
			Content:     content,
			Preexisting: fileExists(targetPath),
		})
	}
	return nil
}

func (p *CopyPlan) collectActions(models []*gen.ModelInfo) ([]moduleCopyAction, error) {
	var aggregate *gen.ModelInfo
	for _, modelInfo := range models {
		if countDesignActions(modelInfo.Design) == 0 {
			continue
		}
		if aggregate != nil {
			return nil, fmt.Errorf("module %s has multiple model designs with actions: %s and %s", p.Name, aggregate.ModelName, modelInfo.ModelName)
		}
		aggregate = modelInfo
	}
	if aggregate == nil {
		return nil, fmt.Errorf("module %s has no aggregate model design with actions", p.Name)
	}

	actions := make([]moduleCopyAction, 0)
	aggregate.Design.Range(func(route string, action *dsl.Action) {
		sourcePath := filepath.Join(p.SourceServiceDir, action.ServiceFilename())
		targetPath := filepath.Join(p.TargetServiceDir, action.ServiceFilename())
		actions = append(actions, moduleCopyAction{
			Route:      route,
			Action:     action,
			SourcePath: sourcePath,
			TargetPath: targetPath,
			MethodName: action.Phase.MethodName(),
			ModelInfo:  p.targetModelInfo(aggregate),
		})
	})
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].TargetPath < actions[j].TargetPath
	})

	for _, action := range actions {
		if err := requireActionSourceFile(action); err != nil {
			return nil, err
		}
	}
	return actions, nil
}

func (p *CopyPlan) targetModelInfo(source *gen.ModelInfo) *gen.ModelInfo {
	// Reuse gg gen's service generator by projecting the framework aggregate model
	// into the current project's model layout. The source model still drives the
	// action DSL; only module/package/path metadata changes.
	target := *source
	target.ModulePath = p.ProjectModulePath
	target.ModelPkgName = p.Name
	target.ModelFileDir = p.TargetModelDir
	target.ModelFilePath = filepath.Join(p.TargetModelDir, filepath.Base(source.ModelFilePath))
	return &target
}

// requireActionSourceFile enforces the module-copy convention that each action
// service file has exactly one service struct and the method matching its DSL
// phase. Helper files are discovered separately by type-based dependency scan.
func requireActionSourceFile(action moduleCopyAction) error {
	if _, err := os.Stat(action.SourcePath); err != nil {
		return fmt.Errorf("source action service file not found for %s: %w", action.Action.ServiceFilename(), err)
	}
	count, err := countServiceStructsInFile(action.SourcePath)
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("source action service file %s must contain exactly one service struct, found %d", action.SourcePath, count)
	}
	if !sourceServiceMethodExists(action.SourcePath, action.MethodName) {
		return fmt.Errorf("source action service file %s has no %s method", action.SourcePath, action.MethodName)
	}
	return nil
}

func (p *CopyPlan) addServiceFiles(helperFiles []string) error {
	for _, action := range p.Actions {
		source, err := os.ReadFile(action.SourcePath)
		if err != nil {
			return err
		}
		target, err := generateTargetServiceShell(action.ModelInfo, action.Action)
		if err != nil {
			return err
		}
		content, err := mergeModuleActionServiceSource(moduleActionMergeInput{
			SourcePath:            action.SourcePath,
			Source:                source,
			TargetPath:            action.TargetPath,
			Target:                target,
			ModuleName:            p.Name,
			TargetModelImportPath: p.TargetModelImportPath,
			MethodName:            action.MethodName,
		})
		if err != nil {
			return err
		}
		p.Files = append(p.Files, moduleCopyFile{
			Kind:        moduleCopyFileService,
			TargetPath:  action.TargetPath,
			Content:     content,
			Preexisting: fileExists(action.TargetPath),
		})
	}

	sort.Strings(helperFiles)
	for _, sourcePath := range helperFiles {
		targetPath := filepath.Join(p.TargetServiceDir, filepath.Base(sourcePath))
		src, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		content, err := normalizeModuleServiceSource(sourcePath, src, p.Name, p.TargetModelImportPath)
		if err != nil {
			return err
		}
		p.Files = append(p.Files, moduleCopyFile{
			Kind:        moduleCopyFileHelper,
			TargetPath:  targetPath,
			Content:     content,
			Preexisting: fileExists(targetPath),
		})
	}
	return nil
}

func (p *CopyPlan) checkConflicts(force bool) error {
	for _, file := range p.Files {
		if !file.Preexisting {
			continue
		}
		old, err := os.ReadFile(file.TargetPath)
		if err != nil {
			return err
		}
		if string(old) == string(file.Content) {
			continue
		}
		if !force {
			return fmt.Errorf("%s already exists; use --force to overwrite", file.TargetPath)
		}
	}
	return nil
}

// ModelTargets returns current-project model files that copy will write.
func (p *CopyPlan) ModelTargets() []string {
	return p.targetsByKind(moduleCopyFileModel)
}

// ServiceTargets returns current-project action service files that copy will merge.
func (p *CopyPlan) ServiceTargets() []string {
	return p.targetsByKind(moduleCopyFileService)
}

// HelperTargets returns current-project helper service files that copy will write.
func (p *CopyPlan) HelperTargets() []string {
	return p.targetsByKind(moduleCopyFileHelper)
}

func (p *CopyPlan) targetsByKind(kind moduleCopyFileKind) []string {
	targets := make([]string, 0)
	for _, file := range p.Files {
		if file.Kind == kind {
			targets = append(targets, file.TargetPath)
		}
	}
	return targets
}

func goFilesInDir(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !isModuleCopyGoSource(info.Name()) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	sort.Strings(files)
	return files, err
}

func isModuleCopyGoSource(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") && !strings.HasPrefix(name, ".")
}

func countDesignActions(design *dsl.Design) int {
	if design == nil {
		return 0
	}
	var count int
	design.Range(func(string, *dsl.Action) {
		count++
	})
	return count
}

func actionSourcePaths(actions []moduleCopyAction) []string {
	paths := make([]string, 0, len(actions))
	seen := make(map[string]bool, len(actions))
	for _, action := range actions {
		if seen[action.SourcePath] {
			continue
		}
		seen[action.SourcePath] = true
		paths = append(paths, action.SourcePath)
	}
	sort.Strings(paths)
	return paths
}

func checkModuleCopyReadyModels(modelRoot string, models []*gen.ModelInfo) error {
	actionDesigns := make(map[string]bool)
	for _, modelInfo := range models {
		if countDesignActions(modelInfo.Design) > 0 {
			actionDesigns[modelInfo.ModelName] = true
		}
	}
	if len(actionDesigns) != 1 {
		return fmt.Errorf("module model source must contain exactly one Design with actions, found %d", len(actionDesigns))
	}

	files, err := goFilesInDir(modelRoot)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := checkModelFileActionDesigns(file, actionDesigns); err != nil {
			return err
		}
	}
	return nil
}

// checkModelFileActionDesigns allows non-aggregate models to keep ordinary DSL
// settings, but rejects action declarations outside the single aggregate model.
func checkModelFileActionDesigns(path string, aggregate map[string]bool) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil || fn.Name.Name != "Design" {
			continue
		}
		recvName := receiverTypeName(fn)
		if recvName == "" || aggregate[recvName] {
			continue
		}
		if designHasActionCall(fn) {
			return fmt.Errorf("non-aggregate Design %s in %s contains action DSL", recvName, path)
		}
	}
	return nil
}

func designHasActionCall(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		found = isActionCall(call)
		return !found
	})
	return found
}

func isActionCall(call *ast.CallExpr) bool {
	name := callName(call.Fun)
	switch name {
	case "Create", "Delete", "Update", "Patch", "List", "Get",
		"CreateMany", "DeleteMany", "UpdateMany", "PatchMany", "Import", "Export":
		return true
	default:
		return false
	}
}

func callName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	default:
		return ""
	}
}

func receiverTypeName(fn *ast.FuncDecl) string {
	if fn == nil || fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	switch typ := fn.Recv.List[0].Type.(type) {
	case *ast.Ident:
		return typ.Name
	case *ast.StarExpr:
		if ident, ok := typ.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func checkModuleNotRegistered(name string) error {
	// Framework-module registration and local-source copy are mutually exclusive:
	// running both would register the same module's model/service/router paths twice.
	moduleFile := filepath.Join("module", "module.go")
	src, err := os.ReadFile(moduleFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, moduleFile, src, parser.ParseComments)
	if err != nil {
		return err
	}

	aliases := make(map[string]bool)
	importPath := filepath.Join(frameworkModulePath, "module", name)
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != importPath {
			continue
		}
		if spec.Name != nil {
			if spec.Name.Name == "." {
				return fmt.Errorf("framework module %s is already imported in %s", name, moduleFile)
			}
			if spec.Name.Name != "_" {
				aliases[spec.Name.Name] = true
			}
			continue
		}
		aliases[pathBase(path)] = true
	}
	if len(aliases) == 0 {
		return nil
	}

	registered := false
	ast.Inspect(file, func(node ast.Node) bool {
		if registered {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Register" {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && aliases[ident.Name] {
			registered = true
			return false
		}
		return true
	})
	if registered {
		return fmt.Errorf("framework module %s is already registered; remove it before copying local source", name)
	}
	return nil
}

func pathBase(path string) string {
	return filepath.Base(filepath.ToSlash(path))
}

func getModuleName() (string, error) {
	content, err := os.ReadFile("go.mod")
	if err != nil {
		return "", fmt.Errorf("failed to read go.mod: %w", err)
	}

	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}

	return "", errors.New("module name not found in go.mod")
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}
