package ggmodule

import (
	"fmt"

	"golang.org/x/tools/go/ast/astutil"
)

// RemoveModule unregisters a built-in framework module from module/module.go.
//
// Removal only deletes the import and the zero-argument Register call managed by
// gg module add. It intentionally does not delete model/service source files;
// local source cleanup belongs to the source-copy flow and is a separate user
// decision.
func RemoveModule(projectDir string, name string) (ChangeResult, error) {
	module, err := moduleForRegistration(name)
	if err != nil {
		return ChangeResult{}, err
	}

	path := projectModuleFile(projectDir)
	fset, file, err := parseGoFile(path)
	if err != nil {
		return ChangeResult{}, err
	}

	alias, registered := registeredModuleAlias(file, module)
	if !registered {
		return ChangeResult{}, fmt.Errorf("module %q is not registered as a framework module", name)
	}
	// registeredModuleAlias accepts any alias.Register(...) call, while removal
	// only touches the zero-argument call gg add emits. Reporting ChangeRemoved
	// without writing the file would claim a change that never happened, so a
	// registration gg cannot manage is an error the user has to resolve.
	if !removeRegisterCall(file, alias) {
		return ChangeResult{}, fmt.Errorf("module %q is registered with a Register call that takes arguments; remove the call and its import from %s manually", name, path)
	}
	astutil.DeleteImport(fset, file, module.ImportPath)
	if err := writeGoFile(path, fset, file); err != nil {
		return ChangeResult{}, err
	}
	return ChangeResult{Module: module, Status: ChangeRemoved, Path: path}, nil
}
