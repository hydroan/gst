package ggmodule

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
)

// CopyExecution applies a previously checked CopyPlan.
type CopyExecution struct {
	Plan         *CopyPlan
	Options      CopyOptions
	RunGen       func() error
	WrittenFiles []string
}

// Run applies the copy in the required order:
// model source first, gg gen second, service/helper business logic last.
// It does not roll back partial writes; the command prints the prune cleanup
// path when a failure happens after files were written.
func (e *CopyExecution) Run() error {
	clioutput.Section("Copy Model Files")
	for _, file := range e.Plan.Files {
		if file.Kind != moduleCopyFileModel {
			continue
		}
		if err := e.write(file); err != nil {
			return err
		}
	}

	if e.RunGen == nil {
		return errors.New("module copy requires a gg gen runner")
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
		switch status {
		case "SKIP":
			clioutput.Item("SKIP", "%s", path)
		case "UPDATE":
			clioutput.Status(clioutput.StyleWarn, clioutput.SymbolSuccess, "UPDATE", "%s", path)
		case "CREATE":
			clioutput.Success("CREATE", "%s", path)
		}
	}

	return nil
}

func (e *CopyExecution) write(file moduleCopyFile) error {
	if file.Kind == moduleCopyFileService || file.Kind == moduleCopyFileHelper {
		safePath, err := pathUnderRoot(file.TargetPath, e.Plan.ServiceDir)
		if err != nil {
			return err
		}
		file.TargetPath = safePath
	}
	if file.Kind == moduleCopyFileModel {
		safePath, err := pathUnderRoot(file.TargetPath, e.Plan.ModelDir)
		if err != nil {
			return err
		}
		file.TargetPath = safePath
	}
	if file.Kind == moduleCopyFileMiddleware {
		safePath, err := pathUnderRoot(file.TargetPath, e.Plan.targetMiddlewareDir())
		if err != nil {
			return err
		}
		file.TargetPath = safePath
	}

	status, wrote, err := writeModuleCopyFile(file.TargetPath, file.Content, file.Preexisting, e.Options.Force)
	if err != nil {
		return err
	}
	switch status {
	case "SKIP":
		clioutput.Item("SKIP", "%s", file.TargetPath)
	case "UPDATE":
		clioutput.Status(clioutput.StyleWarn, clioutput.SymbolSuccess, "UPDATE", "%s", file.TargetPath)
	case "CREATE":
		clioutput.Success("CREATE", "%s", file.TargetPath)
	}
	if wrote {
		e.WrittenFiles = append(e.WrittenFiles, file.TargetPath)
	}
	return nil
}

func writeModuleCopyFile(path string, content []byte, preexisting bool, force bool) (status string, wrote bool, err error) {
	if fileExists(path) {
		oldData, err := os.ReadFile(path)
		if err != nil {
			return "", false, err
		}
		if string(oldData) == string(content) {
			return "SKIP", false, nil
		}
		if preexisting && !force {
			return "", false, fmt.Errorf("%s already exists; use --force to overwrite", path)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			return "", false, err
		}
		return "UPDATE", true, nil
	}

	if err := ensureParentDir(path); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return "", false, err
	}
	return "CREATE", true, nil
}

func ensureParentDir(filename string) error {
	dir := filepath.Dir(filename)

	var err error
	if _, err = os.Stat(dir); err == nil {
		return nil
	} else if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}
	return err
}

// pathUnderRoot returns path cleaned and verified to be under root (no path traversal).
func pathUnderRoot(path, root string) (string, error) {
	path = filepath.Clean(path)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s is not under root %s", path, root)
	}
	return path, nil
}
