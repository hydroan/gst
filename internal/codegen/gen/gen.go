package gen

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/stoewer/go-strcase"
)

// ModelInfo stores model information
//
// Examples:
// {ModulePath:"helloworld", ModelPkgName:"model", ModelName:"User", ModelVarName:"u", ModelFileDir:"model", ModelFilePath:"model/user.go"},
// {ModulePath:"helloworld", ModelPkgName:"sample", ModelName:"Group", ModelVarName:"g", ModelFileDir:"model/sample", ModelFilePath:"model/sample/group.go"},
type ModelInfo struct {
	// module related fields
	ModulePath string // module path parsed from go.mod

	// model related fields
	ModelPkgName  string // model package name, e.g.: model, sample
	ModelName     string // model name, e.g.: User, Group
	ModelVarName  string // lowercase model variable name, e.g.: u, g
	ModelFileDir  string // directory of the model file, relative to the project root, e.g.: model/sample
	ModelFilePath string // path of the model file, relative to the project root, e.g.: model/sample/group.go

	// custom request and response related fields
	Design *dsl.Design

	// RegisterIgnored marks a model matched by a gst.yaml gen.models.ignore
	// rule: its generated model.Register call is skipped while column
	// generation still treats it as a table-backed model.
	RegisterIgnored bool
}

// ServiceTargetInfo locates the service file of an action (see
// ServiceTarget). For a Create action on the model Item of model/sample/item.go
// in module helloworld it holds
//
//	Dir:         "service/sample/item"
//	FilePath:    "service/sample/item/create.go"
//	ImportPath:  "helloworld/service/sample/item"
//	PackageName: "item"
type ServiceTargetInfo struct {
	Dir         string // directory of the service file, under the service directory
	FilePath    string // path of the service file
	ImportPath  string // import path of the service package
	PackageName string // name of the service package
}

// ServiceOutputRel returns the path under the service root where generated service .go files
// for a model file should live, relative to the service directory (e.g. "common" for
// model/common/common.go, or "sample/item/entry" for model/sample/item/entry.go).
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

// ServiceTarget locates the service file of the action on model m under
// serviceDir. By default the file lives in a package of its own named after
// the model, in the directory ServiceOutputRel maps the model file to: a
// Create action on the model Role of model/authz/role.go goes to
// service/authz/role/create.go, in package role. With Flatten the file lives
// in the directory of the model package itself and takes its package name:
// with Filename("role.go") too, the same action goes to service/authz/role.go,
// in package authz.
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

// flattenedServiceOutputRel returns the directory under the service root a
// flattened service file of the model file lives in: the directory of the
// model file relative to modelDir, as in "authz" for model/authz/role.go, or
// "" for a model file in modelDir itself.
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

// ImportPath returns the import path of the package the model is declared in.
func (m *ModelInfo) ImportPath() string {
	return filepath.Join(m.ModulePath, m.ModelFileDir)
}

// InModelRoot reports whether the model is declared in the root model package,
// the directory modelDir itself, which the generated model registration file
// belongs to.
func (m *ModelInfo) InModelRoot(modelDir string) bool {
	return filepath.Clean(m.ModelFileDir) == filepath.Clean(modelDir)
}

// ModelPackageName returns the name a model package in the directory named
// dirName declares, as gg check requires: the directory name with its
// underscores stripped, as in record_item holding package recorditem and
// sample holding package sample.
func ModelPackageName(dirName string) string {
	return strings.ReplaceAll(dirName, "_", "")
}

// GetModulePath returns the module path of the project in the working
// directory: the one go list -m reports, run with workspace mode off, or,
// when that fails, the one the module directive of go.mod declares. It fails
// when the working directory holds no go.mod.
func GetModulePath() (string, error) {
	file, err := os.Open("go.mod")
	if err != nil {
		return "", err
	}
	defer file.Close()

	// If go command exists, get module path directly through go list -m command.
	// Disable workspace mode explicitly: inside a go.work workspace "go list -m"
	// prints every workspace module (one per line) instead of the current one,
	// which would corrupt every generated import path.
	cmd := exec.Command("go", "list", "-m")
	cmd.Env = append(os.Environ(), "GOWORK=off")
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

// isModelBase checks if a struct field is an anonymous embedding of a
// database base model (model.Base or model.AutoBase), handling aliased
// imports of the model package.
func isModelBase(file *ast.File, field *ast.Field) bool {
	// Not anonymous field.
	if len(field.Names) != 0 {
		return false
	}

	aliasName := constants.PkgModel
	if spec := findImportSpec(file, constants.ImportPathModel); spec != nil && spec.Name != nil {
		aliasName = spec.Name.Name
	}

	switch t := field.Type.(type) {
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name == aliasName && (t.Sel.Name == constants.FieldBase || t.Sel.Name == constants.FieldAutoBase)
		}
	case *ast.Ident:
		return t.Name == constants.FieldBase || t.Name == constants.FieldAutoBase
	}

	return false
}

// isModelEmpty checks if a struct field is an anonymous embedding of
// model.Empty, the base of a model without a database table, handling aliased
// imports of the model package.
func isModelEmpty(file *ast.File, field *ast.Field) bool {
	// Not anonymous field.
	if len(field.Names) != 0 {
		return false
	}

	aliasName := constants.PkgModel
	if spec := findImportSpec(file, constants.ImportPathModel); spec != nil && spec.Name != nil {
		aliasName = spec.Name.Name
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

// FindModels returns the models the model file filename declares: its
// structs embedding model.Base, model.AutoBase or model.Empty, each with the
// design its Design method declares (see dsl.Parse). The DSL of the file is
// validated first, and every violation is returned as one error.
func FindModels(module string, modelDir string, filename string) ([]*ModelInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	modelPkgName := node.Name.Name
	if len(modelPkgName) == 0 {
		return nil, fmt.Errorf("file %s has no model package", filename)
	}

	if errs := dsl.Validate(node, modelDir, filename); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	designs := dsl.Parse(node)
	// Note: route assembly (prefixing the model file dir onto Design.Endpoint)
	// lives in cmd/gg/gen.go so custom routes declared in the DSL are handled
	// in one place.

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

// humanizeDSLFilename turns a DSL Filename() value into a space-separated label: underscores
// and hyphens become spaces; consecutive whitespace is collapsed. It returns
// "batch upload" for batch_upload and "export report" for export-report.go.
func humanizeDSLFilename(filename string) string {
	name := filepath.Base(filename)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	s := strings.ReplaceAll(name, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	return strings.Join(strings.Fields(s), " ")
}

// serviceActionLogQuoted returns a Go string literal (as used in ast.BasicLit.Value) for
// log.Info in generated service methods: "{model} {phase}", as in
// "user create" and "user create before". When action.Filename is set, uses
// "{model}: {humanized filename}" with optional hook suffix (before/after/filter/...),
// as in "user: archive before" for Filename("archive").
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
		default:
			msg = fmt.Sprintf("%s: %s", modelLower, label)
		}
		return strconv.Quote(msg)
	}
	msg := fmt.Sprintf("%s %s", modelLower, phaseSnake)
	return strconv.Quote(msg)
}

// genServiceMethod1 uses AST to generate CreateBefore,CreateAfter,UpdateBefore,UpdateAfter,
// DeleteBefore,DeleteAfter,GetBefore,GetAfter,PatchBefore,PatchAfter methods. modelQualifier is
// the name the file refers to the model package by (see serviceModelQualifier). For the model
// User and phase consts.PHASE_CREATE_BEFORE it generates
//
//	func (u *Creator) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create before")
//
//		return nil
//	}
func genServiceMethod1(info *ModelInfo, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod1(
		info.ModelVarName, info.ModelName, modelQualifier, phase, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod2 uses AST to generate ListBefore, ListAfter methods, referring to the
// model package by modelQualifier (see genServiceMethod1). For the model User and phase
// consts.PHASE_LIST_BEFORE it generates
//
//	func (u *Lister) ListBefore(ctx *gst.ServiceContext, users *[]*model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user list before")
//
//		return nil
//	}
func genServiceMethod2(info *ModelInfo, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod2(
		info.ModelVarName, info.ModelName, modelQualifier, phase, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod3 uses AST to generate CreateManyBefore, CreateManyAfter,
// DeleteManyBefore, DeleteManyAfter, UpdateManyBefore, UpdateManyAfter, PatchManyBefore, PatchManyAfter,
// referring to the model package by modelQualifier (see genServiceMethod1). For the model User
// and phase consts.PHASE_CREATE_MANY_BEFORE it generates
//
//	func (u *ManyCreator) CreateManyBefore(ctx *gst.ServiceContext, users ...*model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create many before")
//
//		return nil
//	}
func genServiceMethod3(info *ModelInfo, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod3(
		info.ModelVarName, info.ModelName, modelQualifier, phase, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod4 uses AST to generate Create,Delete,Update,Patch,List,Get,CreateMany,DeleteMany,UpdateMany,PatchMany methods,
// referring to the model package by modelQualifier (see genServiceMethod1). For the model User,
// the request and result types *User and phase consts.PHASE_CREATE it generates
//
//	func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create")
//
//		return rsp, nil
//	}
func genServiceMethod4(info *ModelInfo, modelQualifier string, action *dsl.Action, reqName, rspName string, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod4(
		info.ModelVarName, modelQualifier, reqName, rspName, phase, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(
			ast.NewIdent("rsp"),
			ast.NewIdent("nil"),
		),
	)
}

// genServiceMethod5 uses AST to generate Import method, referring to the model
// package by modelQualifier (see genServiceMethod1). The scaffold returns a
// literal nil error: returning the never-assigned named err would fail the
// service error discipline check on the very next gg run. For the model User
// it generates
//
//	func (u *Importer) Import(ctx *gst.ServiceContext, reader io.Reader) (users []*model.User, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user import")
//
//		return users, nil
//	}
func genServiceMethod5(info *ModelInfo, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod5(
		info.ModelVarName, info.ModelName, modelQualifier, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent(pluralizeCli.Plural(strings.ToLower(info.ModelName))), ast.NewIdent("nil")),
	)
}

// genServiceMethod6 uses AST to generate Export method, referring to the model
// package by modelQualifier (see genServiceMethod1). Like the Import scaffold,
// it returns a literal nil error to keep generated code compliant with the
// service error discipline check. For the model User it generates
//
//	func (u *Exporter) Export(ctx *gst.ServiceContext, users ...*model.User) (data []byte, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user export")
//
//		return data, nil
//	}
func genServiceMethod6(info *ModelInfo, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod6(
		info.ModelVarName, info.ModelName, modelQualifier, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("data"), ast.NewIdent("nil")),
	)
}

// genServiceMethod7 uses AST to generate the SSE method scaffold. Like the
// Import scaffold, it returns a literal nil error to keep generated code
// compliant with the service error discipline check; the business fills in
// the streaming callback through ctx.SSE. For the model User it generates
//
//	func (u *Streamer) SSE(ctx *gst.ServiceContext) (err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user sse")
//
//		return nil
//	}
func genServiceMethod7(info *ModelInfo, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod7(
		info.ModelVarName, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// GenerateService builds the scaffold of the action's service file in package
// servicePkgName: the service struct named after the action's role and the
// methods of phase. It returns nil when the action is disabled or declares no
// service. For a Create action on the model User of the root model package,
// a database model, the file prints as
//
//	package user
//
//	import (
//		"helloworld/model"
//
//		"github.com/hydroan/gst"
//		"github.com/hydroan/gst/service"
//	)
//
//	type Creator struct {
//		service.Base[*model.User, *model.User, *model.User]
//	}
//
//	func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create")
//
//		return rsp, nil
//	}
//
//	func (u *Creator) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create before")
//
//		return nil
//	}
//
//	func (u *Creator) CreateAfter(ctx *gst.ServiceContext, user *model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create after")
//
//		return nil
//	}
//
// A model without a database table, one embedding model.Empty, gets no
// before and after hooks.
func GenerateService(info *ModelInfo, action *dsl.Action, phase consts.Phase, servicePkgName string) *ast.File {
	if !action.Enabled || !action.Service {
		return nil
	}

	roleName := action.RoleName()

	// When Filename is set, derive the receiver variable name from RoleName
	// (e.g., Archive → "a", Publish → "p") instead of the model name (e.g., Record → "r").
	if len(action.Filename) > 0 && len(roleName) > 0 {
		copied := *info
		copied.ModelVarName = strings.ToLower(roleName[:1])
		info = &copied
	}

	// The file refers to the model package by its package name, or by an
	// alias when a framework package the file imports takes that name, as in
	// service.Base[*model_service.Item, ...] (see serviceModelQualifier).
	qualifier := serviceModelQualifier(info, phase)

	otherPkgs := []string{}
	// A dsl.PayloadEmpty request or result type references model.Empty from
	// the gst model package, so the generated file needs its import (aliased
	// when the file refers to the business model package as "model").
	if isEmptyPayload(action.Payload) || isEmptyPayload(action.Result) {
		otherPkgs = append(otherPkgs, emptyReqImport(qualifier))
	}

	decls := []ast.Decl{
		imports(info.ModulePath, info.ModelFileDir, qualifier, phase, otherPkgs...),
		types(qualifier, info.ModelName, action.Payload, action.Result, roleName),
	}

	// add methods
	switch phase {
	case consts.PHASE_CREATE:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
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
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.Before(), roleName)) // generate create before hook
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.After(), roleName))  // generate create after hook
		}
	case consts.PHASE_DELETE:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.Before(), roleName)) // generate delete before hook
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.After(), roleName))  // generate delete after hook
		}
	case consts.PHASE_UPDATE:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.Before(), roleName)) // generate update before hook
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.After(), roleName))  // generate update after hook
		}
	case consts.PHASE_PATCH:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.Before(), roleName)) // generate patch before hook
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.After(), roleName))  // generate patch after hook
		}
	case consts.PHASE_LIST: // List hooks use genServiceMethod2
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod2(info, qualifier, action, phase.Before(), roleName)) // generate list before hook
			decls = append(decls, genServiceMethod2(info, qualifier, action, phase.After(), roleName))  // generate list after hook
		}
	case consts.PHASE_GET:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.Before(), roleName)) // generate get before hook
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.After(), roleName))  // generate get after hook
		}
	case consts.PHASE_CREATE_MANY: // XXXMany hooks use genServiceMethod3
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.Before(), roleName)) // generate create many before hook
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.After(), roleName))  // generate create many after hook
		}
	case consts.PHASE_DELETE_MANY:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.Before(), roleName)) // generate delete many before hook
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.After(), roleName))  // generate delete many after hook
		}
	case consts.PHASE_UPDATE_MANY:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.Before(), roleName)) // generate update many before hook
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.After(), roleName))  // generate update many after hook
		}
	case consts.PHASE_PATCH_MANY:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.Before(), roleName)) // generate patch many before hook
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.After(), roleName))  // generate patch many after hook
		}
	case consts.PHASE_IMPORT:
		decls = append(decls, genServiceMethod5(info, qualifier, action, phase, roleName))
	case consts.PHASE_SSE:
		decls = append(decls, genServiceMethod7(info, action, phase, roleName))
	case consts.PHASE_EXPORT:
		// The export controller reuses the list pipeline before delegating to
		// Export: it invokes ListBefore, applies the service Filter hook when
		// building the query, then invokes ListAfter. Filter has a pass-through
		// default, so only the Before/After hooks are scaffolded here.
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod2(info, qualifier, action, consts.PHASE_LIST_BEFORE, roleName)) // generate list before hook
			decls = append(decls, genServiceMethod2(info, qualifier, action, consts.PHASE_LIST_AFTER, roleName))  // generate list after hook
		}
		decls = append(decls, genServiceMethod6(info, qualifier, action, phase, roleName))
	}

	return &ast.File{
		Name:  ast.NewIdent(servicePkgName),
		Decls: decls,
	}
}
