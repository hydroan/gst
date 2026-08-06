package ggmodule

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// ChangeStatus describes whether a module command changed module/module.go.
type ChangeStatus string

// The statuses AddModule and RemoveModule report to the command layer.
// ChangeCreated means module/module.go gained the framework import and the
// Register call, ChangeRemoved means both were deleted, and ChangeSkipped means
// the file already matched the request and was left untouched.
const (
	ChangeCreated ChangeStatus = "created"
	ChangeRemoved ChangeStatus = "removed"
	ChangeSkipped ChangeStatus = "skipped"
)

// ChangeResult is returned by add/remove commands so the Cobra layer can decide
// how to present the operation without knowing AST details.
type ChangeResult struct {
	Module Module
	Status ChangeStatus
	Path   string
}

// moduleForRegistration applies the command-level constraints shared by add and
// remove. A module can be listed even when it is not addable, but automatic
// registration only works when the module's Register function can be called as
// pkg.Register() with no arguments.
func moduleForRegistration(name string) (Module, error) {
	if err := validateModuleCommandName(name, "module command"); err != nil {
		return Module{}, err
	}
	module, err := moduleByName(name)
	if os.IsNotExist(err) {
		return Module{}, fmt.Errorf("module %q not found", name)
	}
	if err != nil {
		return Module{}, err
	}
	if !module.Addable {
		return Module{}, fmt.Errorf("module %q cannot be added automatically because Register requires arguments", name)
	}
	return module, nil
}

// validateModuleCommandName keeps module commands on catalog entries instead of
// arbitrary filesystem paths. This avoids ambiguous commands such as `gg module
// add module/copytest` and prevents path traversal from reaching outside the
// module catalog. subject names the command in the path errors, so add/remove
// and copy each report the contract the user actually invoked.
func validateModuleCommandName(name string, subject string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("module name is required")
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("module name %q must not contain surrounding whitespace", name)
	}
	if strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%s accepts a module name, not a path: %s", subject, name)
	}
	if filepath.Clean(name) != name || filepath.Base(name) != name {
		return fmt.Errorf("%s accepts a module name, not a path: %s", subject, name)
	}
	return nil
}

func projectModuleFile(projectDir string) string {
	return filepath.Join(projectDir, "module", "module.go")
}
