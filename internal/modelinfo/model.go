package modelinfo

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
)

// Model stores model information
//
// Examples:
// {ModulePath:"helloworld", ModelPkgName:"model", ModelName:"User", ModelVarName:"u", ModelFileDir:"model", ModelFilePath:"model/user.go"},
// {ModulePath:"helloworld", ModelPkgName:"sample", ModelName:"Group", ModelVarName:"g", ModelFileDir:"model/sample", ModelFilePath:"model/sample/group.go"},
type Model struct {
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
// for a model file should live, relative to the service directory (e.g. "sample" for
// model/sample/sample.go, or "sample/item/entry" for model/sample/item/entry.go).
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
// with Service("role") too, the same action goes to service/authz/role.go,
// in package authz.
func ServiceTarget(m *Model, action *dsl.Action, modelDir, serviceDir string) ServiceTargetInfo {
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
		ImportPath:  PackageImportPath(m.ModulePath, dir),
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

// ServiceConflicts reports, for each action of models generating a service,
// the service file, type or package it shares with an action found earlier,
// in the order the models were found and their actions are declared: gg gen
// writes each service where ServiceTarget puts it, so two actions put in one
// file overwrite each other, and two declaring one type in a directory, or
// two packages in one directory, leave a project that does not build. For
// model/record/item.go declaring Create with Flatten() and Service("create")
// on Item, found first, and model/record.go declaring Create with Service()
// on Record, it returns
//
//	model/record.go: the Create action of Record writes its service to service/record/create.go, as the Create action of Item in model/record/item.go does; a service file serves one action, so give each a Service("name") of its own
//
// for the Create actions of Record under Route("sample/archive") and
// Route("sample/seal") in model/sample/record.go naming their services
// Service("item_archive") and Service("itemArchive")
//
//	model/sample/record.go: the Create action of Record under Route("sample/seal") declares the service type ItemArchive in service/sample/record, as the Create action of Record under Route("sample/archive") in model/sample/record.go does; a package declares a name once, so give each a Service("name") of its own
//
// and for model/sample/record.go declaring Get with Service() on Record and
// then List with Service() on Item, whose services both go to
// service/sample/record
//
//	model/sample/record.go: the List action of Item writes package item into service/sample/record, where the Get action of Record in model/sample/record.go writes package record; a directory holds one package
//
// An action conflicting in more than one way is reported for each, but for
// a type it shares with the action whose file it shares as well.
func ServiceConflicts(models []*Model) []error {
	type service struct {
		file, action, pkg string
	}
	type typeKey struct {
		dir, name string
	}
	files := make(map[string]service)
	types := make(map[typeKey]service)
	packages := make(map[string]service)
	var conflicts []error
	for _, m := range models {
		if m.Design == nil {
			continue
		}
		m.Design.Range(func(route string, act *dsl.Action) {
			if !act.Service {
				return
			}
			target := ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
			action := "the " + act.Phase.Name() + " action of " + m.ModelName
			if route != m.Design.Endpoint {
				action += fmt.Sprintf(" under Route(%q)", route)
			}
			current := service{file: m.ModelFilePath, action: action, pkg: target.PackageName}
			declared := typeKey{dir: target.Dir, name: act.RoleName()}
			var found []error
			fileOwner, sharesFile := files[target.FilePath]
			if sharesFile {
				found = append(found, errors.Newf("%s: %s writes its service to %s, as %s in %s does; a service file serves one action, so give each a Service(\"name\") of its own",
					current.file, current.action, target.FilePath, fileOwner.action, fileOwner.file))
			}
			if typeOwner, ok := types[declared]; ok && (!sharesFile || typeOwner != fileOwner) {
				found = append(found, errors.Newf("%s: %s declares the service type %s in %s, as %s in %s does; a package declares a name once, so give each a Service(\"name\") of its own",
					current.file, current.action, declared.name, target.Dir, typeOwner.action, typeOwner.file))
			}
			if packageOwner, ok := packages[target.Dir]; ok && packageOwner.pkg != current.pkg {
				found = append(found, errors.Newf("%s: %s writes package %s into %s, where %s in %s writes package %s; a directory holds one package",
					current.file, current.action, current.pkg, target.Dir, packageOwner.action, packageOwner.file, packageOwner.pkg))
			}
			if len(found) > 0 {
				conflicts = append(conflicts, found...)
				return
			}
			files[target.FilePath] = current
			types[declared] = current
			if _, ok := packages[target.Dir]; !ok {
				packages[target.Dir] = current
			}
		})
	}
	return conflicts
}

// ImportPath returns the import path of the package the model is declared in
// (see PackageImportPath).
func (m *Model) ImportPath() string {
	return PackageImportPath(m.ModulePath, m.ModelFileDir)
}

// PackageImportPath returns the import path of the project package in dir, a
// directory relative to the module root, with forward slashes whatever the
// host writes paths with: in module tmpapp, model/sample gives
// tmpapp/model/sample, and the module root, "." or "", gives tmpapp.
func PackageImportPath(module string, dir string) string {
	dir = strings.Trim(filepath.ToSlash(filepath.Clean(dir)), "/")
	if dir == "" || dir == "." {
		return module
	}
	return module + "/" + dir
}

// InModelRoot reports whether the model is declared in the root model package,
// the directory modelDir itself, which the generated model registration file
// belongs to.
func (m *Model) InModelRoot(modelDir string) bool {
	return filepath.Clean(m.ModelFileDir) == filepath.Clean(modelDir)
}

// ModelPackageName returns the name a model package in the directory named
// dirName declares, as gg check requires: the directory name with its
// underscores stripped, as in record_item holding package recorditem and
// sample holding package sample.
func ModelPackageName(dirName string) string {
	return strings.ReplaceAll(dirName, "_", "")
}

// findModelsInFile returns the models the model file filename declares: its
// structs embedding model.Base, model.AutoBase or model.Empty, each with the
// design its Design method declares (see dsl.Parse). The DSL of the file is
// validated first, and every violation is returned as one error.
func findModelsInFile(module string, modelDir string, filename string) ([]*Model, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	modelPkgName := node.Name.Name
	if len(modelPkgName) == 0 {
		return nil, errors.Newf("file %s has no model package", filename)
	}

	if errs := dsl.Validate(node, modelDir, filename); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	designs := dsl.Parse(node)
	// Route assembly (prefixing the model file dir onto Design.Endpoint) lives
	// in ResolveRoutes, so custom routes declared in the DSL are handled in one
	// place.

	var models []*Model
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
				if dsl.IsModelBase(node, field) || dsl.IsModelEmpty(node, field) {
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
			models = append(models, &Model{
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
