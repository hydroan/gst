package main

import (
	"cmp"
	"go/types"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen/ts"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelsource"
	"github.com/spf13/cobra"
	"golang.org/x/tools/go/packages"
)

// typeScriptDir is where gg gen ts writes, under the generated directory gg
// migrate writes its schemas to.
var typeScriptDir = filepath.Join("generated", "typescript")

var tsCmd = &cobra.Command{
	Use:   "ts",
	Short: "Generate TypeScript declarations of the API types",
	Long: `Generate the TypeScript declarations of the types the API routes declared in
model Design() send and receive, into generated/typescript: a file per Go
package of the model directory, mirroring its tree, plus a file named after the
application holding the response envelope and the default list and batch shapes.
A type a model borrows from elsewhere in the project keeps its own path. The
files hold types only and refer to each other through relative imports, so the
directory can be copied into any frontend as it is.

The directory is kept in step with the models: what an earlier run generated for
a model that is now gone is removed.

The routes registered at run time are covered too, the ones module.Use
registers from the project's module package and the ones of the framework
modules that package registers, through gg module add or by hand: the types of
a framework module go where a copy of the module would put them, iam/account.ts
for the accounts of the iam module.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return genTypeScriptRun()
	},
}

func init() {
	genCmd.AddCommand(tsCmd)
}

// genTypeScriptRun generates the TypeScript declarations. It runs the project
// checks, all but Generated files, and reads the models the way gg gen does,
// so the declarations cover exactly the routes gg gen registers for the
// models as they are, and reads the project's module package for the routes
// registered at run time (see typeScriptModuleRoots). It ends on what it
// changed under the output directory.
func genTypeScriptRun() error {
	started := time.Now()
	if len(module) == 0 {
		var err error
		if module, err = gghelper.ModulePath("."); err != nil {
			return err
		}
	}
	// The declarations come from the models, whether or not gg gen has
	// written the Go files of the project since they changed.
	if err := runProjectChecks(projectChecksWithout(ggcheck.GeneratedFiles.Name), false, nil); err != nil {
		return errors.Wrap(err, "nothing was generated")
	}
	// The declarations mirror the models. A project with no model directory
	// declares no route at all, and the run then removes what an earlier one
	// generated rather than leaving stale declarations behind.
	var models []*modelsource.Model
	if gghelper.FileExists(ggconst.DirModel) {
		projectCfg, err := loadProjectConfig()
		if err != nil {
			return err
		}
		scanned, err := scanModels(false, gghelper.NewProjectIgnore(), projectCfg)
		if err != nil {
			return err
		}
		reportIgnoreWarnings(scanned)
		models = scanned.models
	}
	appName, err := applicationName()
	if err != nil {
		return err
	}
	moduleRoots, err := typeScriptModuleRoots(module)
	if err != nil {
		return errors.Wrap(err, "load the module package (run gg gen first if generated files are out of date)")
	}

	files, err := ts.Generate(ts.Config{
		Dir:           ".",
		ModulePath:    module,
		RootPath:      path.Join(module, filepath.ToSlash(ggconst.DirModel)),
		DeclaredTrees: typeScriptDeclaredTrees(moduleRoots),
		AppName:       appName,
		Roots:         uniqueTypeScriptRoots(append(typeScriptRoots(models), moduleRoots...)),
	})
	var diagnostics *ts.DiagnosticsError
	switch {
	case errors.As(err, &diagnostics):
		return err
	case err != nil:
		return errors.Wrap(err, "load the model packages (run gg gen first if generated files are out of date)")
	}
	log := new(changes)
	if err = writeTypeScriptFiles(typeScriptDir, files, log); err != nil {
		return err
	}

	summary := log.summary()
	switch {
	case len(files) == 0:
		clioutput.Done("No route declares a type, so %s holds nothing (%s)", typeScriptDir, elapsedSince(started))
	case summary == "":
		clioutput.Done("Nothing to generate: every declaration in %s is up to date (%s)", typeScriptDir, elapsedSince(started))
	default:
		clioutput.Done("Generated in %s: %s (%s)", typeScriptDir, summary, elapsedSince(started))
	}
	return nil
}

// applicationName returns the name the project configured, which names the file
// the framework prelude goes to. gg reads the project configuration the way gg
// migrate does; a project that configured no name falls back to the framework
// name inside the generator.
func applicationName() (string, error) {
	if err := config.Init(); err != nil {
		return "", errors.Wrap(err, "read the project configuration")
	}
	return config.App.AppInfo.Name, nil
}

// typeScriptRoots returns the types the routes of models exchange as JSON: the
// Payload and Result types of every enabled action, the messages of a Stream
// among them. Import and Export move files and SSE streams events, so their
// types never travel as JSON, and *model.Empty carries no data.
func typeScriptRoots(models []*modelsource.Model) []ts.TypeRef {
	var roots []ts.TypeRef
	for _, m := range models {
		pkgPath := m.ImportPath()
		m.Design.Range(func(_ string, action *dsl.Action) {
			if dsl.HTTPOnlyAction(action.Phase.Name()) {
				return
			}
			for _, typeName := range []string{action.Payload, action.Result} {
				if typeName == "" || typeName == dsl.PayloadEmpty {
					continue
				}
				roots = append(roots, ts.TypeRef{PkgPath: pkgPath, Name: strings.TrimPrefix(typeName, "*")})
			}
		})
	}
	return uniqueTypeScriptRoots(roots)
}

// uniqueTypeScriptRoots returns roots sorted by package and name, each once.
func uniqueTypeScriptRoots(roots []ts.TypeRef) []ts.TypeRef {
	slices.SortFunc(roots, func(a, b ts.TypeRef) int {
		return cmp.Or(strings.Compare(a.PkgPath, b.PkgPath), strings.Compare(a.Name, b.Name))
	})
	return slices.Compact(roots)
}

// Where the framework keeps what its modules are made of, by import path: the
// module package, whose Use registers a route and whose tree holds a package
// per module, and the tree of the modules' models.
const (
	frameworkModuleTree = ggconst.ImportPathGst + "/" + ggconst.DirModule
	funcModuleUse       = "Use"
	frameworkModelTree  = ggconst.ImportPathGst + "/" + ggconst.DirModuleModel
)

// typeScriptDeclaredTrees returns the package trees outside the project whose
// types get declarations: the tree of the framework's module models, and each
// framework module package declaring one of roots itself, as
// github.com/hydroan/gst/module/helloworld does for the roots of the
// helloworld module. A module package that only forwards the types of its
// models declares none, and stays a package read from compiled export data.
func typeScriptDeclaredTrees(roots []ts.TypeRef) []string {
	trees := []string{frameworkModelTree}
	for _, root := range roots {
		if strings.HasPrefix(root.PkgPath, frameworkModuleTree+"/") && !slices.Contains(trees, root.PkgPath) {
			trees = append(trees, root.PkgPath)
		}
	}
	return trees
}

// typeScriptModuleRoots returns the types the routes registered at run time
// exchange as JSON: the request and response types of every module.Use in
// the project's module package, and in every framework module package whose
// Register that package refers to, which is how gg module add registers a
// module. A project without a module package registers none. modulePath is
// the project's module path.
//
// For a module package holding
//
//	func init() {
//		helloworld.Register()
//	}
//
// it returns the types module/helloworld passes to module.Use: Req, Rsp and
// Helloworld2 of github.com/hydroan/gst/module/helloworld.
func typeScriptModuleRoots(modulePath string) ([]ts.TypeRef, error) {
	if !hasGoSources(ggconst.DirModule) {
		return nil, nil
	}
	// The packages holding the calls are type-checked from source, for the
	// type arguments of the calls; what they import is read from compiled
	// export data.
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir: ".",
	}
	loaded, err := packages.Load(cfg, path.Join(modulePath, ggconst.DirModule))
	if err != nil {
		return nil, err
	}
	var registered []string
	for _, pkg := range loaded {
		for _, obj := range pkg.TypesInfo.Uses {
			if fn, ok := obj.(*types.Func); ok && fn.Name() == ggconst.FuncRegister && fn.Pkg() != nil &&
				strings.HasPrefix(fn.Pkg().Path(), frameworkModuleTree+"/") {
				registered = append(registered, fn.Pkg().Path())
			}
		}
	}
	if len(registered) > 0 {
		slices.Sort(registered)
		modules, loadErr := packages.Load(cfg, slices.Compact(registered)...)
		if loadErr != nil {
			return nil, loadErr
		}
		loaded = append(loaded, modules...)
	}

	var roots []ts.TypeRef
	for _, pkg := range loaded {
		if len(pkg.Errors) > 0 {
			return nil, errors.Newf("%s does not type-check: %s", pkg.PkgPath, pkg.Errors[0].Error())
		}
		for id, instance := range pkg.TypesInfo.Instances {
			fn, ok := pkg.TypesInfo.Uses[id].(*types.Func)
			if !ok || fn.Name() != funcModuleUse || fn.Pkg() == nil || fn.Pkg().Path() != frameworkModuleTree {
				continue
			}
			// The type arguments of module.Use: the model, the request and
			// the response.
			for i := 1; i < instance.TypeArgs.Len(); i++ {
				if ref, isRoot := typeScriptRootOf(instance.TypeArgs.At(i)); isRoot {
					roots = append(roots, ref)
				}
			}
		}
	}
	return uniqueTypeScriptRoots(roots), nil
}

// typeScriptRootOf returns the root a request or response type argument of
// module.Use names: the type declared at package scope it is, or points to,
// as {PkgPath: "github.com/hydroan/gst/module/helloworld", Name: "Req"} for
// *helloworld.Req. An alias a framework module package declares forwards a
// type of the module's models, and the root is the type it forwards:
// {PkgPath: "github.com/hydroan/gst/internal/modulemodel/authz", Name: "Role"}
// for *authz.Role. A type that carries no data (see isEmptyModel) is no root,
// and neither is one written out without a name of its own, which has no
// declaration to name.
func typeScriptRootOf(t types.Type) (ts.TypeRef, bool) {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	for {
		alias, ok := t.(*types.Alias)
		if !ok || alias.Obj().Pkg() == nil || !strings.HasPrefix(alias.Obj().Pkg().Path(), frameworkModuleTree+"/") {
			break
		}
		t = alias.Rhs()
	}
	var obj *types.TypeName
	switch tt := t.(type) {
	case *types.Alias:
		obj = tt.Obj()
	case *types.Named:
		obj = tt.Obj()
	default:
		return ts.TypeRef{}, false
	}
	if isEmptyModel(t) || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
		return ts.TypeRef{}, false
	}
	return ts.TypeRef{PkgPath: obj.Pkg().Path(), Name: obj.Name()}, true
}

// isEmptyModel reports whether t is model.Empty, or a struct holding nothing
// beside it: what a module names as the request of a route that reads no
// body, which carries no data.
func isEmptyModel(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	// The type carries the name a model embeds it under.
	if obj := named.Obj(); obj.Pkg() != nil && obj.Pkg().Path() == ggconst.ImportPathInternalModel && obj.Name() == ggconst.FieldEmpty {
		return true
	}
	fields, ok := named.Underlying().(*types.Struct)
	if !ok || fields.NumFields() == 0 {
		return false
	}
	for field := range fields.Fields() {
		if !field.Embedded() || !isEmptyModel(field.Type()) {
			return false
		}
	}
	return true
}

// writeTypeScriptFiles writes files under dir, skipping the unchanged ones, and
// removes what an earlier run generated that no route needs any more, so the
// directory mirrors the models, recording in log what it wrote and removed.
// A file gg did not generate is never touched: one in the way of an output
// file stops the run, one anywhere else is kept and reported.
func writeTypeScriptFiles(dir string, files []ts.File, log *changes) error {
	header := ggconst.CodeGeneratedComment
	wanted := make(map[string]bool, len(files))
	for _, f := range files {
		target := filepath.Join(dir, filepath.FromSlash(f.Path))
		wanted[target] = true
		content, err := os.ReadFile(target)
		switch {
		case os.IsNotExist(err):
		case err != nil:
			return errors.Wrapf(err, "read %s", target)
		case !strings.HasPrefix(string(content), header):
			return errors.Newf("%s was not generated by gg; move it out of %s", target, dir)
		}
	}
	for _, f := range files {
		if err := writeGeneratedFile(filepath.Join(dir, filepath.FromSlash(f.Path)), f.Content, log); err != nil {
			return err
		}
	}
	return removeOrphanTypeScriptFiles(dir, wanted, header, log)
}

// removeOrphanTypeScriptFiles removes the generated .ts files under dir the
// current run did not write, and the directories that leaves empty, dir
// included: a project that generates nothing keeps no output directory. A file
// without the generated header is kept and reported; the ones removed are
// recorded in log.
func removeOrphanTypeScriptFiles(dir string, wanted map[string]bool, header string, log *changes) error {
	if !gghelper.FileExists(dir) {
		return nil
	}
	var dirs []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		if wanted[path] || filepath.Ext(path) != ".ts" {
			return nil
		}
		content, err := os.ReadFile(path) //nolint:gosec // path comes from walking the output directory.
		if err != nil {
			return errors.Wrapf(err, "read %s", path)
		}
		if !strings.HasPrefix(string(content), header) {
			clioutput.Warn("%s is kept: it was not generated by gg", path)
			return nil
		}
		// #nosec G122 -- path comes from walking the output directory, and only
		// files carrying the generated header reach here.
		if err = os.Remove(path); err != nil {
			return errors.Wrapf(err, "remove %s", path)
		}
		log.record(clioutput.LabelRemove, path, " (no route uses its types any more)")
		return nil
	})
	if err != nil {
		return err
	}
	// The walk lists a directory before its subdirectories, so going backwards
	// empties the children before their parents are looked at.
	for _, d := range slices.Backward(dirs) {
		if entries, readErr := os.ReadDir(d); readErr == nil && len(entries) == 0 {
			if err = os.Remove(d); err != nil {
				return errors.Wrapf(err, "remove %s", d)
			}
		}
	}
	return nil
}
