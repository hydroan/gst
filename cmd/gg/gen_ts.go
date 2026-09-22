package main

import (
	"cmp"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/codegen/gen/ts"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/spf13/cobra"
)

// typeScriptDir is where gg gen ts writes, under the generated directory gg
// migrate writes its schemas to.
var typeScriptDir = filepath.Join("generated", "typescript")

var tsCmd = &cobra.Command{
	Use:   "ts",
	Short: "generate TypeScript declarations of the API types",
	Long: `Generate the TypeScript declarations of the types the API routes declared in
model Design() send and receive, into generated/typescript: a file per Go
package of the model directory, mirroring its tree, plus a file named after the
application holding the response envelope and the default list and batch shapes.
A type a model borrows from elsewhere in the project keeps its own path. The
files hold types only and refer to each other through relative imports, so the
directory can be copied into any frontend as it is.

The directory is kept in step with the models: what an earlier run generated for
a model that is now gone is removed.

Routes registered at runtime, through gg module add or module.Use, are not
covered.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := genTypeScriptRun(); err != nil {
			clioutput.Error("", "%v", err)
			os.Exit(1)
		}
	},
}

func init() {
	genCmd.AddCommand(tsCmd)
}

// genTypeScriptRun generates the TypeScript declarations. It runs the project
// checks and reads the models the way gg gen does, so the declarations cover
// exactly the routes the generated router registers.
func genTypeScriptRun() error {
	if len(module) == 0 {
		var err error
		if module, err = gen.GetModulePath(); err != nil {
			return err
		}
	}
	if runProjectChecks(false, nil) > 0 {
		return errors.New("project checks failed")
	}
	// The declarations mirror the models. A project with no model directory
	// declares no route at all, and the run then removes what an earlier one
	// generated rather than leaving stale declarations behind.
	var models []*gen.ModelInfo
	if fileExists(ggconst.DirModel) {
		scanned, err := scanModels(false)
		if err != nil {
			return err
		}
		models = scanned.models
	}
	appName, err := applicationName()
	if err != nil {
		return err
	}

	clioutput.Section("Generate TypeScript")
	files, err := ts.Generate(ts.Config{
		Dir:        ".",
		ModulePath: module,
		RootPath:   path.Join(module, filepath.ToSlash(ggconst.DirModel)),
		AppName:    appName,
		Roots:      typeScriptRoots(models),
	})
	var diagnostics *ts.DiagnosticsError
	switch {
	case errors.As(err, &diagnostics):
		return err
	case err != nil:
		return errors.Wrap(err, "load the model packages (run gg gen first if generated files are out of date)")
	}
	if err = writeTypeScriptFiles(typeScriptDir, files); err != nil {
		return err
	}

	clioutput.Section("Done")
	if len(files) == 0 {
		clioutput.Done("No route declares a type, so %s holds nothing", typeScriptDir)
		return nil
	}
	clioutput.Done("TypeScript declarations generated in %s", typeScriptDir)
	return nil
}

// applicationName returns the name the project configured, which names the file
// the framework prelude goes to. gg reads the project configuration the way gg
// migrate does; a project that configured no name falls back to the framework
// name inside the generator.
func applicationName() (string, error) {
	defer config.Clean()

	if err := config.Init(); err != nil {
		return "", errors.Wrap(err, "read the project configuration")
	}
	return config.App.AppInfo.Name, nil
}

// typeScriptRoots returns the types the routes of models exchange as JSON: the
// Payload and Result types of every enabled action. Import and Export move
// files and SSE streams events, so their types never travel as JSON, and
// *model.Empty carries no data.
func typeScriptRoots(models []*gen.ModelInfo) []ts.TypeRef {
	seen := make(map[ts.TypeRef]bool)
	var roots []ts.TypeRef
	for _, m := range models {
		pkgPath := m.ImportPath()
		m.Design.Range(func(_ string, action *dsl.Action) {
			switch action.Phase {
			case consts.PHASE_IMPORT, consts.PHASE_EXPORT, consts.PHASE_SSE:
				return
			}
			for _, typeName := range []string{action.Payload, action.Result} {
				if typeName == "" || typeName == dsl.PayloadEmpty {
					continue
				}
				ref := ts.TypeRef{PkgPath: pkgPath, Name: strings.TrimPrefix(typeName, "*")}
				if !seen[ref] {
					seen[ref] = true
					roots = append(roots, ref)
				}
			}
		})
	}
	slices.SortFunc(roots, func(a, b ts.TypeRef) int {
		return cmp.Or(strings.Compare(a.PkgPath, b.PkgPath), strings.Compare(a.Name, b.Name))
	})
	return roots
}

// writeTypeScriptFiles writes files under dir, skipping the unchanged ones, and
// removes what an earlier run generated that no route needs any more, so the
// directory mirrors the models. A file gg did not generate is never touched: one
// in the way of an output file stops the run, one anywhere else is kept and
// reported.
func writeTypeScriptFiles(dir string, files []ts.File) error {
	header := consts.CodeGeneratedComment()
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
		if err := writeGeneratedFile(filepath.Join(dir, filepath.FromSlash(f.Path)), f.Content, true); err != nil {
			return err
		}
	}
	return removeOrphanTypeScriptFiles(dir, wanted, header)
}

// removeOrphanTypeScriptFiles removes the generated .ts files under dir the
// current run did not write, and the directories that leaves empty, dir
// included: a project that generates nothing keeps no output directory. A file
// without the generated header is kept and reported.
func removeOrphanTypeScriptFiles(dir string, wanted map[string]bool, header string) error {
	if !fileExists(dir) {
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
			clioutput.Warn("KEEP", "%s was not generated by gg", path)
			return nil
		}
		// #nosec G122 -- path comes from walking the output directory, and only
		// files carrying the generated header reach here.
		if err = os.Remove(path); err != nil {
			return errors.Wrapf(err, "remove %s", path)
		}
		clioutput.Success("REMOVE", "%s (no route uses its types any more)", path)
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
