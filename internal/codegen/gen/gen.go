package gen

import (
	"bufio"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/types/consts"
	"github.com/stoewer/go-strcase"
)

// ModelInfo stores model information
//
// Examples:
// {ModulePath:"github.com/hydroan/gst", ModelPkgName:"model", ModelName:"User", ModelVarName:"u", ModelFileDir:"/tmp/model"},
// {ModulePath:"github.com/hydroan/gst", ModelPkgName:"model", ModelName:"Group", ModelVarName:"g", ModelFileDir:"/tmp/model"},
// {ModulePath:"github.com/hydroan/gst", ModelPkgName:"model_auth", ModelName:"User", ModelVarName:"u", ModelFileDir:"/tmp/model"},
// {ModulePath:"github.com/hydroan/gst", ModelPkgName:"model_auth", ModelName:"Group", ModelVarName:"g", ModelFileDir:"/tmp/model"},
type ModelInfo struct {
	// module related fields
	ModulePath string // module path parsed from go.mod

	// model related fields
	ModelPkgName  string // model package name, e.g.: model, model_authz, model_log
	ModelName     string // model name, e.g.: User, Group
	ModelVarName  string // lowercase model variable name, e.g.: u, g
	ModelFileDir  string // relative path of model file directory, e.g.: github.com/hydroan/gst/model
	ModelFilePath string // relative path of model file, e.g.: github.com/hydroan/gst/model/user.go

	// custom request and response related fields
	Design *dsl.Design
}

type ServiceTargetInfo struct {
	Dir         string
	FilePath    string
	ImportPath  string
	PackageName string
}

// ServiceOutputRel returns the path under the service root where generated service .go files
// for a model file should live, relative to the service directory (e.g. "common" for
// model/common/common.go, or "config/namespace/app/env/item" for model/.../env/item.go).
//
// When the file base name (without .go) equals the immediate parent directory name — a common
// Go layout such as model/pkg/pkg.go — redundant segments are collapsed so output is
// service/pkg/... instead of service/pkg/pkg/...
func ServiceOutputRel(modelFilePath, modelDir string) string {
	modelDir = filepath.Clean(modelDir)
	modelFilePath = filepath.Clean(modelFilePath)
	rel, err := filepath.Rel(modelDir, modelFilePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		// Unexpected layout; best-effort: strip modelDir prefix then apply the same collapse.
		rel = strings.TrimPrefix(modelFilePath, modelDir+string(filepath.Separator))
	}
	outRel := strings.TrimSuffix(rel, ".go")
	for outRel != "." && outRel != "" {
		stem := filepath.Base(outRel)
		parent := filepath.Dir(outRel)
		if parent == "." || parent == "" {
			break
		}
		if filepath.Base(parent) == stem {
			outRel = parent
			continue
		}
		break
	}
	return outRel
}

func (m *ModelInfo) ServiceImportPath(modelDir, serviceDir string) string {
	rel := ServiceOutputRel(m.ModelFilePath, modelDir)
	return filepath.Join(m.ModulePath, serviceDir, rel)
}

func ServiceTarget(m *ModelInfo, action *dsl.Action, modelDir, serviceDir string) ServiceTargetInfo {
	rel := ServiceOutputRel(m.ModelFilePath, modelDir)
	packageName := strings.ToLower(m.ModelName)
	if action != nil && action.Flatten {
		rel = flattenedServiceOutputRel(m.ModelFilePath, modelDir)
		packageName = m.ModelPkgName
	}

	dir := filepath.Join(serviceDir, rel)
	return ServiceTargetInfo{
		Dir:         dir,
		FilePath:    filepath.Join(dir, action.ServiceFilename()),
		ImportPath:  filepath.Join(m.ModulePath, serviceDir, rel),
		PackageName: packageName,
	}
}

func flattenedServiceOutputRel(modelFilePath, modelDir string) string {
	modelDir = filepath.Clean(modelDir)
	modelFilePath = filepath.Clean(modelFilePath)
	rel, err := filepath.Rel(modelDir, modelFilePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		rel = strings.TrimPrefix(modelFilePath, modelDir+string(filepath.Separator))
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		return ""
	}
	return dir
}

func (m *ModelInfo) RouterImportPath() string {
	return filepath.Join(m.ModulePath, m.ModelFileDir)
}

func (m *ModelInfo) ModelImportPath() (string, bool) {
	// If a struct anonymous inherits from model.Base, than the model will be imported in model/model.go using
	// statement such like: "model.Register[*User]()".
	// Imported the model is not determinated by m.Design.Eanbled value.
	path := filepath.Join(m.ModulePath, m.ModelFileDir)
	if !strings.HasSuffix(path, "/model") {
		return path, true
	}
	return "", false
}

// GetModulePath parses go.mod to get module path
func GetModulePath() (string, error) {
	file, err := os.Open("go.mod")
	if err != nil {
		return "", err
	}
	defer file.Close()

	// If go command exists, get module path directly through go list -m command
	cmd := exec.Command("go", "list", "-m")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output)), nil
	}

	var moduleName string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module") {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				moduleName = parts[1]
			}
		}
	}

	return moduleName, scanner.Err()
}

// findModelPackageName finds the actual name of the imported model package
// import "github.com/hydroan/gst/model" returns "model"
// import model_auth "github.com/hydroan/gst/model" returns model_auth
func findModelPackageName(file *ast.File) string {
	return file.Name.Name
}

// // isModelBase 检查字段是否是 model.Base
//
//	func isModelBase(file *ast.File, field *ast.Field, modelPkgName string) bool {
//		if field.Names != nil { // 不是匿名字段
//			return false
//		}
//
//		getAliasName := func(file *ast.File) string {
//			for _, imp := range file.Imports {
//				path := strings.Trim(imp.Path.Value, `"`)
//				if strings.HasSuffix(path, "github.com/hydroan/gst/model") {
//					if imp.Name != nil {
//						return imp.Name.Name // 使用重命名的包名
//					}
//					return "model" // 默认包名
//				}
//			}
//			return ""
//		}
//		aliasName := getAliasName(file)
//
//		switch t := field.Type.(type) {
//		case *ast.SelectorExpr:
//			if ident, ok := t.X.(*ast.Ident); ok {
//				return ident.Name == aliasName && t.Sel.Name == "Base"
//			}
//		case *ast.Ident:
//			// 处理同包的情况
//			return t.Name == "Base"
//		}
//
//		return false
//	}
func isModelBase(file *ast.File, field *ast.Field) bool {
	// Not anonymouse field.
	if len(field.Names) != 0 {
		return false
	}

	aliasName := constants.PkgModel
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		if imp.Path.Value == constants.ModelPackagePath {
			if imp.Name != nil {
				aliasName = imp.Name.Name
			}
			break
		}
	}

	switch t := field.Type.(type) {
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name == aliasName && t.Sel.Name == constants.FieldBase
		}
	case *ast.Ident:
		return t.Name == constants.FieldBase
	}

	return false
}

func isModelEmpty(file *ast.File, field *ast.Field) bool {
	// Not anonymouse field.
	if len(field.Names) != 0 {
		return false
	}

	aliasName := constants.PkgModel
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		if imp.Path.Value == constants.ModelPackagePath {
			if imp.Name != nil {
				aliasName = imp.Name.Name
			}
			break
		}
	}

	switch t := field.Type.(type) {
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name == aliasName && t.Sel.Name == constants.FieldEmpty
		}
	case *ast.Ident:
		return t.Name == constants.FieldEmpty
	}

	return false
}

// FindModels finds all structs in model files
func FindModels(module string, modelDir string, filename string) ([]*ModelInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	modelPkgName := findModelPackageName(node)
	if len(modelPkgName) == 0 {
		return nil, fmt.Errorf("file %s has no model package", filename)
	}
	f, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	if errs := dsl.Validate(f, modelDir, filename); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	designs := dsl.Parse(f, "")
	// Note: Endpoint assembly logic has been moved to concatEndpoints function in cmd/gg/gen.go
	// to properly handle custom endpoints defined in DSL
	//
	// for _, design := range designs {
	// 	// The new endpoint value is the model file dir + the endpoint value
	// 	// For example: old endpoint is "order", the model dir is "model/user",
	// 	// then the new endpoint is "user/order"
	// 	newFilename := strings.TrimPrefix(filename, modelDir) // "/user/order.go"
	// 	newFilename = strings.TrimPrefix(newFilename, "/")    // "user/order.go"
	// 	dir := filepath.Dir(newFilename)                      // "user"
	// 	design.Endpoint = filepath.Join(dir, design.Endpoint) // "user/order"
	// }

	var models []*ModelInfo
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl == nil || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec == nil || typeSpec.Type == nil {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType == nil || structType.Fields == nil {
				continue
			}
			hasModel := false
			for _, field := range structType.Fields.List {
				if isModelBase(node, field) || isModelEmpty(node, field) {
					hasModel = true
					break
				}
			}
			if !hasModel || typeSpec.Name == nil {
				continue
			}
			modelName := typeSpec.Name.Name
			if len(modelName) == 0 {
				continue
			}
			models = append(models, &ModelInfo{
				ModelFileDir:  filepath.Dir(filename),
				ModelFilePath: filename,
				ModelPkgName:  modelPkgName,
				ModelName:     modelName,
				ModelVarName:  strings.ToLower(modelName[:1]),
				ModulePath:    module,
				Design:        designs[modelName],
			})

		}
	}

	return models, nil
}

// modelPkg2ServicePkg converts model name to service name.
func modelPkg2ServicePkg(pkgName string) string {
	if pkgName == constants.PkgModel {
		return constants.PkgService
	}
	// For model_xxx format, replace with service_xxx
	modelPrefix := constants.PrefixModel + constants.SeparatorUnderscore
	servicePrefix := constants.PrefixService + constants.SeparatorUnderscore
	if strings.HasPrefix(pkgName, modelPrefix) {
		return strings.Replace(pkgName, modelPrefix, servicePrefix, 1)
	}
	return strings.Replace(pkgName, constants.PrefixModel, constants.PrefixService, 1)
}

// humanizeDSLFilename turns a DSL Filename() value into a space-separated label: underscores
// and hyphens become spaces; consecutive whitespace is collapsed.
func humanizeDSLFilename(filename string) string {
	name := filepath.Base(filename)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	s := strings.ReplaceAll(name, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	return strings.Join(strings.Fields(s), " ")
}

// serviceActionLogQuoted returns a Go string literal (as used in ast.BasicLit.Value) for
// log.Info in generated service methods. When action.Filename is set, uses
// "{model}: {humanized filename}" with optional hook suffix (before/after/filter/...).
func serviceActionLogQuoted(modelName string, phase consts.Phase, action *dsl.Action) string {
	modelLower := strings.ToLower(modelName)
	phaseSnake := strings.ReplaceAll(strcase.SnakeCase(phase.MethodName()), "_", " ")
	if action != nil && len(action.Filename) > 0 {
		label := humanizeDSLFilename(action.Filename)
		ps := string(phase)
		var msg string
		switch {
		case strings.HasSuffix(ps, "_before"):
			msg = fmt.Sprintf("%s: %s before", modelLower, label)
		case strings.HasSuffix(ps, "_after"):
			msg = fmt.Sprintf("%s: %s after", modelLower, label)
		case phase == consts.PHASE_FILTER:
			msg = fmt.Sprintf("%s: %s filter", modelLower, label)
		case phase == consts.PHASE_FILTER_RAW:
			msg = fmt.Sprintf("%s: %s filter raw", modelLower, label)
		default:
			msg = fmt.Sprintf("%s: %s", modelLower, label)
		}
		return strconv.Quote(msg)
	}
	msg := fmt.Sprintf("%s %s", modelLower, phaseSnake)
	return strconv.Quote(msg)
}

// genServiceMethod1 uses AST to generate CreateBefore,CreateAfter,UpdateBefore,UpdateAfter,
// DeleteBefore,DeleteAfter,GetBefore,GetAfter,PatchBefore,PatchAfter methods.
func genServiceMethod1(info *ModelInfo, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod1(
		info.ModelVarName, info.ModelName, info.ModelPkgName, phase, roleName,
		StmtLogWithServiceContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod2 uses AST to generate ListBefore, ListAfter methods.
func genServiceMethod2(info *ModelInfo, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod2(
		info.ModelVarName, info.ModelName, info.ModelPkgName, phase, roleName,
		StmtLogWithServiceContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod3 uses AST to generate CreateManyBefore, CreateManyAfter,
// DeleteManyBefore, DeleteManyAfter, UpdateManyBefore, UpdateManyAfter, PatchManyBefore, PatchManyAfter.
func genServiceMethod3(info *ModelInfo, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod3(
		info.ModelVarName, info.ModelName, info.ModelPkgName, phase, roleName,
		StmtLogWithServiceContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod4 uses AST to generate Create,Delete,Update,Patch,List,Get,CreateMany,DeleteMany,UpdateMany,PatchMany methods.
func genServiceMethod4(info *ModelInfo, action *dsl.Action, reqName, rspName string, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod4(
		info.ModelVarName, info.ModelName, info.ModelPkgName, reqName, rspName, phase, roleName,
		StmtLogWithServiceContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(
			ast.NewIdent("rsp"),
			ast.NewIdent("nil"),
		),
	)
}

// genServiceMethod5 uses AST to generate Import method.
func genServiceMethod5(info *ModelInfo, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod5(
		info.ModelVarName, info.ModelName, info.ModelPkgName, phase, roleName,
		StmtLogWithServiceContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent(pluralizeCli.Plural(strings.ToLower(info.ModelName))), ast.NewIdent("err")),
	)
}

// genServiceMethod6 uses AST to generate Export method.
func genServiceMethod6(info *ModelInfo, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod6(
		info.ModelVarName, info.ModelName, info.ModelPkgName, phase, roleName,
		StmtLogWithServiceContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("data"), ast.NewIdent("err")),
	)
}

// genServiceMethod7 uses AST to generate Filter method.
func genServiceMethod7(info *ModelInfo, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod7(
		info.ModelVarName, info.ModelName, info.ModelPkgName, phase, roleName,
		Returns(ast.NewIdent(strings.ToLower(info.ModelName))),
	)
}

// genServiceMethod8 uses AST to generate FilterRaw method.
func genServiceMethod8(info *ModelInfo, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod8(
		info.ModelVarName, info.ModelName, info.ModelPkgName, phase, roleName,
		Returns(ast.NewIdent(`""`)),
	)
}

func GenerateService(info *ModelInfo, action *dsl.Action, phase consts.Phase) *ast.File {
	return GenerateServiceWithPackage(info, action, phase, strings.ToLower(info.ModelName))
}

func GenerateServiceWithPackage(info *ModelInfo, action *dsl.Action, phase consts.Phase, servicePkgName string) *ast.File {
	if !action.Enabled || !action.Service {
		return nil
	}

	roleName := action.RoleName()

	// When Filename is set, derive the receiver variable name from RoleName
	// (e.g., Upload → "u", Parse → "p") instead of the model name (e.g., Attachment → "a").
	if len(action.Filename) > 0 && len(roleName) > 0 {
		copied := *info
		copied.ModelVarName = strings.ToLower(roleName[:1])
		info = &copied
	}

	otherPkgs := []string{}
	if phase == consts.PHASE_IMPORT {
		otherPkgs = append(otherPkgs, "io")
	}

	decls := []ast.Decl{
		imports(info.ModulePath, info.ModelFileDir, info.ModelPkgName, otherPkgs...),
		// Inits(info.ModelName),
		// Types(info.ModelPkgName, info.ModelName, info.Design.Create.Payload, info.Design.Create.Result),
	}

	// add types
	if action.Enabled {
		decls = append(decls, types(info.ModelPkgName, info.ModelName, action.Payload, action.Result, phase, roleName, false))
	}

	// add methods
	switch phase {
	case consts.PHASE_CREATE:
		decls = append(decls, genServiceMethod4(info, action, action.Payload, action.Result, phase, roleName))
		// Hook generation logic based on model.Empty field presence:
		//
		// Models WITHOUT hooks (contains model.Empty):
		// 	type Group struct {
		// 		model.Empty
		// 	}
		//
		// Models WITH hooks (does not contain model.Empty):
		// 	type Group struct {
		// 		Name string
		// 		model.Base
		// 	}
		//
		// Only generate before/after hooks for non-empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, action, phase.Before(), roleName)) // generate create before hook
			decls = append(decls, genServiceMethod1(info, action, phase.After(), roleName))  // generate create after hook
		}
	case consts.PHASE_DELETE:
		decls = append(decls, genServiceMethod4(info, action, action.Payload, action.Result, phase, roleName))
		// Skip generate hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, action, phase.Before(), roleName)) // generate delete before hook
			decls = append(decls, genServiceMethod1(info, action, phase.After(), roleName))  // generate delete after hook
		}
	case consts.PHASE_UPDATE:
		decls = append(decls, genServiceMethod4(info, action, action.Payload, action.Result, phase, roleName))
		// Skip generate hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, action, phase.Before(), roleName)) // generate update before hook
			decls = append(decls, genServiceMethod1(info, action, phase.After(), roleName))  // generate update after hook
		}
	case consts.PHASE_PATCH:
		decls = append(decls, genServiceMethod4(info, action, action.Payload, action.Result, phase, roleName))
		// Skip generate hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, action, phase.Before(), roleName)) // generate patch before hook
			decls = append(decls, genServiceMethod1(info, action, phase.After(), roleName))  // generate patch after hook
		}
	case consts.PHASE_LIST: // List method use GenerateServiceMethod2
		decls = append(decls, genServiceMethod4(info, action, action.Payload, action.Result, phase, roleName))
		// Skip generate hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod2(info, action, phase.Before(), roleName))  // generate list before hook
			decls = append(decls, genServiceMethod2(info, action, phase.After(), roleName))   // generate list after hook
			decls = append(decls, genServiceMethod7(info, consts.PHASE_FILTER, roleName))     // generate filter hook
			decls = append(decls, genServiceMethod8(info, consts.PHASE_FILTER_RAW, roleName)) // generate filter raw hook
		}
	case consts.PHASE_GET:
		decls = append(decls, genServiceMethod4(info, action, action.Payload, action.Result, phase, roleName))
		// Skip generate hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, action, phase.Before(), roleName)) // generate get before hook
			decls = append(decls, genServiceMethod1(info, action, phase.After(), roleName))  // generate get after hook
		}
	case consts.PHASE_CREATE_MANY: // XXXMany methods use GenerateServiceMethod3
		decls = append(decls, genServiceMethod4(info, action, action.Payload, action.Result, phase, roleName))
		// Skip generate hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, action, phase.Before(), roleName)) // generate create many before hook
			decls = append(decls, genServiceMethod3(info, action, phase.After(), roleName))  // generate create many after hook
		}
	case consts.PHASE_DELETE_MANY:
		decls = append(decls, genServiceMethod4(info, action, action.Payload, action.Result, phase, roleName))
		// Skip generate hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, action, phase.Before(), roleName)) // generate delete many before hook
			decls = append(decls, genServiceMethod3(info, action, phase.After(), roleName))  // generate delete many after hook
		}
	case consts.PHASE_UPDATE_MANY:
		decls = append(decls, genServiceMethod4(info, action, action.Payload, action.Result, phase, roleName))
		// Skip generate hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, action, phase.Before(), roleName)) // generate update many before hook
			decls = append(decls, genServiceMethod3(info, action, phase.After(), roleName))  // generate update many after hook
		}
	case consts.PHASE_PATCH_MANY:
		decls = append(decls, genServiceMethod4(info, action, action.Payload, action.Result, phase, roleName))
		// Skip generate hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, action, phase.Before(), roleName)) // generate patch many before hook
			decls = append(decls, genServiceMethod3(info, action, phase.After(), roleName))  // generate patch many after hook
		}
	case consts.PHASE_IMPORT:
		decls = append(decls, genServiceMethod5(info, action, phase, roleName))
	case consts.PHASE_EXPORT:
		decls = append(decls, genServiceMethod6(info, action, phase, roleName))
	}

	return &ast.File{
		Name:  ast.NewIdent(servicePkgName),
		Decls: decls,
	}
}
