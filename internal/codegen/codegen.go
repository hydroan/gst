package codegen

import (
	"go/ast"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/codegen/constants"
	"github.com/hydroan/gst/internal/codegen/gen"
)

// FindModels finds all model infos in a directory
func FindModels(module, modelDir, serviceDir string, excludes []string) ([]*gen.ModelInfo, error) {
	allModels := make([]*gen.ModelInfo, 0)

	if err := filepath.Walk(modelDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		base := filepath.Base(path)
		if path != modelDir && (base == constants.DirVendor || base == constants.DirTestData) {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), constants.ExtensionGo) ||
			strings.HasSuffix(info.Name(), constants.PatternTestFile) ||
			strings.HasPrefix(info.Name(), constants.PrefixIgnore) ||
			slices.Contains(excludes, info.Name()) {
			return nil
		}

		models, err := gen.FindModels(module, modelDir, path)
		if err != nil {
			return err
		}
		for _, m := range models {
			m.ModelFilePath = path
			allModels = append(allModels, m)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return allModels, nil
}

// HasMethod checks if a struct has a specific method
func HasMethod(file *ast.File, structName, methodName string) bool {
	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
				// Check receiver type
				recv := funcDecl.Recv.List[0]
				var recvTypeName string

				switch recvType := recv.Type.(type) {
				case *ast.Ident:
					recvTypeName = recvType.Name
				case *ast.StarExpr:
					if ident, ok := recvType.X.(*ast.Ident); ok {
						recvTypeName = ident.Name
					}
				}

				// Check if this is the method we're looking for
				if recvTypeName == structName && funcDecl.Name.Name == methodName {
					return true
				}
			}
		}
	}
	return false
}

// FindServiceStruct finds the service struct that inherits from service.Base[*Model]
func FindServiceStruct(file *ast.File, modelName string) *ast.TypeSpec {
	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range genDecl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					if structType, ok := typeSpec.Type.(*ast.StructType); ok {
						// Check if this struct embeds service.Base[*ModelName]
						for _, field := range structType.Fields.List {
							if len(field.Names) == 0 { // Embedded field
								if IsServiceBaseType(field.Type, modelName) {
									return typeSpec
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// IsServiceBaseType checks if the type is service.Base[*ModelName]
func IsServiceBaseType(expr ast.Expr, modelName string) bool {
	if indexExpr, ok := expr.(*ast.IndexExpr); ok {
		// Check if X is service.Base
		if selectorExpr, ok := indexExpr.X.(*ast.SelectorExpr); ok {
			if ident, ok := selectorExpr.X.(*ast.Ident); ok && ident.Name == "service" {
				if selectorExpr.Sel.Name == "Base" {
					// Check if the type parameter is *ModelName
					if starExpr, ok := indexExpr.Index.(*ast.StarExpr); ok {
						// Handle qualified names like model_cmdb.DNS
						switch x := starExpr.X.(type) {
						case *ast.Ident:
							return x.Name == modelName
						case *ast.SelectorExpr:
							return x.Sel.Name == modelName
						}
					}
				}
			}
		}
	}
	return false
}
