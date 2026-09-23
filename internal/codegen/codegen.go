package codegen

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/structdoc"
)

// walkModelFiles walks modelDir and invokes fn for every Go source file that
// participates in code generation: it skips the paths the project's Git
// ignore rules exclude, the directories gghelper.ExcludedDir names (hidden,
// vendor and testdata directories and nested modules), test files, ignored
// files (whose names start with "_") and the file names excludes lists. Under
// model it visits model/sample/record.go, and skips model/sample/record_test.go,
// model/sample/_draft.go and every file of model/sample/testdata.
//
// WalkModelFiles exports it: gg check holds the model files to the DSL rules
// through the same walk, so the check and the generator read the same files.
func walkModelFiles(modelDir string, ignore gghelper.ProjectIgnore, excludes []string, fn func(path string) error) error {
	return ignore.Walk(modelDir, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if gghelper.ExcludedDir(modelDir, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ggconst.ExtensionGo) ||
			strings.HasSuffix(info.Name(), ggconst.PatternTestFile) ||
			strings.HasPrefix(info.Name(), ggconst.PrefixIgnore) ||
			slices.Contains(excludes, info.Name()) {
			return nil
		}

		return fn(path)
	})
}

// WalkModelFiles invokes fn for every model file that participates in code
// generation (see walkModelFiles), so the checks read the same files the
// generator does.
func WalkModelFiles(modelDir string, ignore gghelper.ProjectIgnore, fn func(path string) error) error {
	return walkModelFiles(modelDir, ignore, nil, fn)
}

// FindModels returns the models declared by the Go files under modelDir that
// participate in code generation (see walkModelFiles), each carrying the path
// of its model file (see gen.FindModels). A file that fails to parse or
// declares an invalid DSL fails the whole call.
func FindModels(module, modelDir string, ignore gghelper.ProjectIgnore) ([]*gen.ModelInfo, error) {
	allModels := make([]*gen.ModelInfo, 0)

	if err := walkModelFiles(modelDir, ignore, nil, func(path string) error {
		models, err := gen.FindModels(module, modelDir, path)
		if err != nil {
			return err
		}
		allModels = append(allModels, models...)

		return nil
	}); err != nil {
		return nil, err
	}

	return allModels, nil
}

// ExtractAPIDocs extracts the struct doc comments and enum declarations of
// every exported type declared under modelDir, including custom request and
// response types; unexported types never reach the API surface and are
// skipped. Each entry is keyed by the import path of its package, module
// followed by the directory, as in helloworld/model/sample for
// model/sample/record.go in module helloworld. Enum constants may live in a
// different file than their type declaration; entries of the same package are
// merged, so a type Status declared in status.go gathers the values its
// constants in values.go declare. The returned entries are sorted by package
// path and type name so generated output stays deterministic.
func ExtractAPIDocs(module, modelDir string, ignore gghelper.ProjectIgnore, excludes []string) (gen.APIDocEntries, error) {
	var entries gen.APIDocEntries
	enumByKey := make(map[string]*gen.EnumDocEntry)
	enumKeys := make([]string, 0)

	if err := walkModelFiles(modelDir, ignore, excludes, func(path string) error {
		docs, err := structdoc.ParseFileDocs(path)
		if err != nil {
			return err
		}

		pkgPath := module + "/" + filepath.ToSlash(filepath.Dir(path))
		for typeName, doc := range docs.Structs {
			entries.Structs = append(entries.Structs, gen.StructDocEntry{
				PkgPath:  pkgPath,
				TypeName: typeName,
				Doc:      doc,
			})
		}
		for typeName, doc := range docs.Enums {
			key := pkgPath + "." + typeName
			entry, ok := enumByKey[key]
			if !ok {
				entry = &gen.EnumDocEntry{PkgPath: pkgPath, TypeName: typeName}
				enumByKey[key] = entry
				enumKeys = append(enumKeys, key)
			}
			if entry.Doc.Comment == "" {
				entry.Doc.Comment = doc.Comment
			}
			entry.Doc.Values = append(entry.Doc.Values, doc.Values...)
		}

		return nil
	}); err != nil {
		return gen.APIDocEntries{}, err
	}

	// Only named types with declared constant values are enums.
	sort.Strings(enumKeys)
	for _, key := range enumKeys {
		if entry := enumByKey[key]; len(entry.Doc.Values) > 0 {
			entries.Enums = append(entries.Enums, *entry)
		}
	}

	sort.Slice(entries.Structs, func(i, j int) bool {
		if entries.Structs[i].PkgPath != entries.Structs[j].PkgPath {
			return entries.Structs[i].PkgPath < entries.Structs[j].PkgPath
		}
		return entries.Structs[i].TypeName < entries.Structs[j].TypeName
	})

	return entries, nil
}
