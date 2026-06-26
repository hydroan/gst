package ggmodule

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/tools/go/ast/astutil"
)

// AddModule registers a built-in framework module in the current project's module/module.go.
//
// This is the framework-module registration mode. It does not copy model or
// service source into the project. Keeping it separate from Copy keeps the two
// ownership models explicit:
//   - add/remove manages a framework import plus pkg.Register() call.
//   - copy creates local source files that the project owns afterwards.
func AddModule(projectDir string, name string) (ChangeResult, error) {
	module, err := moduleForRegistration(name)
	if err != nil {
		return ChangeResult{}, err
	}
	err = checkNoLocalSource(projectDir, name)
	if err != nil {
		return ChangeResult{}, err
	}

	path := projectModuleFile(projectDir)
	fset, file, err := parseGoFile(path)
	if err != nil {
		return ChangeResult{}, err
	}

	_, registered := registeredModuleAlias(file, module)
	if registered {
		return ChangeResult{Module: module, Status: ChangeSkipped, Path: path}, nil
	}

	alias, hasImport := existingModuleAlias(file, module)
	changed := false
	if !hasImport {
		// Use the Go parser/printer path instead of string replacement because
		// module/module.go may already contain grouped imports, aliases, comments,
		// or additional init statements. astutil keeps the import block valid, and
		// the final go/format pass normalizes spacing.
		importName := importAliasForModule(module)
		changed = astutil.AddNamedImport(fset, file, identName(importName), module.ImportPath)
		alias = module.PackageName
		if importName != nil {
			alias = importName.Name
		}
	}
	if ensureRegisterCall(file, alias) {
		changed = true
	}
	if !changed {
		return ChangeResult{Module: module, Status: ChangeSkipped, Path: path}, nil
	}
	if err := writeGoFile(path, fset, file); err != nil {
		return ChangeResult{}, err
	}
	return ChangeResult{Module: module, Status: ChangeCreated, Path: path}, nil
}

// checkNoLocalSource prevents mixing the two ownership modes for the same
// module. If model/copytest or service/copytest already exists locally, registering the
// framework's copytest module would create two competing implementations with the
// same conceptual name.
func checkNoLocalSource(projectDir string, name string) error {
	for _, dir := range []string{
		filepath.Join(projectDir, defaultModelDir, name),
		filepath.Join(projectDir, defaultServiceDir, name),
	} {
		hasGoFiles, err := hasModuleGoFiles(dir)
		if err != nil {
			return err
		}
		if hasGoFiles {
			return fmt.Errorf("module %q already exists as local source; remove local model/service files before adding framework module", name)
		}
	}
	return nil
}

func hasModuleGoFiles(root string) (bool, error) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if isModuleCopyGoSource(info.Name()) {
			return errStopWalk
		}
		return nil
	})
	if err == nil || os.IsNotExist(err) {
		return false, nil
	}
	if errors.Is(err, errStopWalk) {
		return true, nil
	}
	return false, err
}

var errStopWalk = errors.New("stop walking")
