package ggmodule

import (
	"fmt"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
)

// moduleCopyWriteStatus is what one write did to a target file. It is a named
// type so a misspelled status cannot silently fall through the print switch.
type moduleCopyWriteStatus string

const (
	moduleCopyWriteSkip   moduleCopyWriteStatus = "SKIP"
	moduleCopyWriteUpdate moduleCopyWriteStatus = "UPDATE"
	moduleCopyWriteCreate moduleCopyWriteStatus = "CREATE"
	moduleCopyWriteDelete moduleCopyWriteStatus = "DELETE"
)

func printModuleCopyStatus(status moduleCopyWriteStatus, path string) {
	switch status {
	case moduleCopyWriteSkip:
		clioutput.Item(string(status), "%s", path)
	case moduleCopyWriteUpdate:
		clioutput.Status(clioutput.StyleWarn, clioutput.SymbolSuccess, string(status), "%s", path)
	case moduleCopyWriteCreate:
		clioutput.Success(string(status), "%s", path)
	case moduleCopyWriteDelete:
		clioutput.Status(clioutput.StyleWarn, clioutput.SymbolWarn, string(status), "%s", path)
	}
}

// CopyExecution applies a previously checked CopyPlan.
type CopyExecution struct {
	Plan         *CopyPlan
	Options      CopyOptions
	RunGen       func() error
	WrittenFiles []string
	DeletedFiles []string
}

// Run applies the copy in the required order: model source first, stale-file
// prune second, gg gen third, service/helper business logic fourth, and
// manifest-declared middleware files plus their registration in
// middleware/middleware.go last.
// It does not roll back partial writes or deletes; the command prints the
// cleanup path when a failure happens after the project was touched.
func (e *CopyExecution) Run() error {
	// Checked before the first write: the run needs gg gen between the model and
	// service phases, so a missing runner must fail while the project is still
	// untouched rather than after model files have landed.
	if e.RunGen == nil {
		return errors.New("module copy requires a gg gen runner")
	}

	clioutput.Section("Copy Model Files")
	for _, file := range e.Plan.Files {
		if file.Kind != moduleCopyFileModel {
			continue
		}
		if err := e.write(file); err != nil {
			return err
		}
	}

	if err := e.pruneStaleFiles(); err != nil {
		return err
	}

	if err := e.RunGen(); err != nil {
		return err
	}

	clioutput.Section("Copy Service Files")
	for _, file := range e.Plan.Files {
		if file.Kind != moduleCopyFileService {
			continue
		}
		if err := e.write(file); err != nil {
			return err
		}
	}

	helperFiles := e.Plan.HelperTargets()
	if len(helperFiles) > 0 {
		clioutput.Section("Copy Helper Files")
		for _, file := range e.Plan.Files {
			if file.Kind != moduleCopyFileHelper {
				continue
			}
			if err := e.write(file); err != nil {
				return err
			}
		}
	}

	if len(e.Plan.Middleware) > 0 {
		clioutput.Section("Copy Middleware Files")
		for _, file := range e.Plan.Files {
			if file.Kind != moduleCopyFileMiddleware {
				continue
			}
			if err := e.write(file); err != nil {
				return err
			}
		}

		clioutput.Section("Register Middleware")
		status, path, err := e.registerMiddleware()
		if err != nil {
			return err
		}
		printModuleCopyStatus(status, path)
	}

	return nil
}

// pruneStaleFiles deletes target files an older copy produced that the current
// plan no longer does, keeping copied directories in sync with the framework
// module source. It runs before gg gen on purpose: a stale model file still
// carries Design() DSL that gen would faithfully regenerate registrations for,
// and a stale service file referencing a removed model would surface as a
// fresh project-check violation during the copy-time gen run.
func (e *CopyExecution) pruneStaleFiles() error {
	staleModelFiles := e.Plan.StaleModelTargets()
	staleServiceFiles := e.Plan.StaleServiceTargets()
	if len(staleModelFiles) == 0 && len(staleServiceFiles) == 0 {
		return nil
	}

	clioutput.Section("Prune Stale Files")
	for _, group := range []struct {
		files []string
		root  string
	}{
		{staleModelFiles, e.Plan.ModelDir},
		{staleServiceFiles, e.Plan.ServiceDir},
	} {
		for _, path := range group.files {
			if err := e.remove(path, group.root); err != nil {
				return err
			}
		}
	}
	return nil
}

// remove deletes one stale file after the same path-traversal check writes go
// through. A file that is already gone counts as pruned: the desired state is
// absence, and a parallel cleanup must not fail the copy.
func (e *CopyExecution) remove(path string, root string) error {
	safePath, err := requirePathUnderRoot(path, root)
	if err != nil {
		return err
	}
	if err := os.Remove(safePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	printModuleCopyStatus(moduleCopyWriteDelete, safePath)
	e.DeletedFiles = append(e.DeletedFiles, safePath)
	return nil
}

func (e *CopyExecution) write(file moduleCopyFile) error {
	if file.Kind == moduleCopyFileService || file.Kind == moduleCopyFileHelper {
		safePath, err := requirePathUnderRoot(file.TargetPath, e.Plan.ServiceDir)
		if err != nil {
			return err
		}
		file.TargetPath = safePath
	}
	if file.Kind == moduleCopyFileModel {
		safePath, err := requirePathUnderRoot(file.TargetPath, e.Plan.ModelDir)
		if err != nil {
			return err
		}
		file.TargetPath = safePath
	}
	if file.Kind == moduleCopyFileMiddleware {
		safePath, err := requirePathUnderRoot(file.TargetPath, e.Plan.TargetMiddlewareDir)
		if err != nil {
			return err
		}
		file.TargetPath = safePath
	}

	status, wrote, err := writeModuleCopyFile(file.TargetPath, file.Content, file.Preexisting, e.Options.Force)
	if err != nil {
		return err
	}
	printModuleCopyStatus(status, file.TargetPath)
	if wrote {
		e.WrittenFiles = append(e.WrittenFiles, file.TargetPath)
	}
	return nil
}

func writeModuleCopyFile(path string, content []byte, preexisting bool, force bool) (status moduleCopyWriteStatus, wrote bool, err error) {
	if fileExists(path) {
		oldData, err := os.ReadFile(path)
		if err != nil {
			return "", false, err
		}
		if string(oldData) == string(content) {
			return moduleCopyWriteSkip, false, nil
		}
		if preexisting && !force {
			return "", false, fmt.Errorf("%s already exists; use --force to overwrite", path)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			return "", false, err
		}
		return moduleCopyWriteUpdate, true, nil
	}

	if err := ensureParentDir(path); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return "", false, err
	}
	return moduleCopyWriteCreate, true, nil
}
