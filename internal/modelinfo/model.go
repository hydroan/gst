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
// with Filename("role.go") too, the same action goes to service/authz/role.go,
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
func (m *Model) ImportPath() string {
	return filepath.Join(m.ModulePath, m.ModelFileDir)
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
		return nil, fmt.Errorf("file %s has no model package", filename)
	}

	if errs := dsl.Validate(node, modelDir, filename); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	designs := dsl.Parse(node)
	// Note: route assembly (prefixing the model file dir onto Design.Endpoint)
	// lives in cmd/gg/gen.go so custom routes declared in the DSL are handled
	// in one place.

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
