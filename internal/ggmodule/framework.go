package ggmodule

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cockroachdb/errors"
)

const frameworkModulePath = "github.com/hydroan/gst"

// findFrameworkRoot resolves the directory holding the framework source for
// the current working directory's project.
//
// The Go module graph is the single authority: `go list -m` resolves the
// framework to whatever the project builds against — the module cache for a
// plain require directive, the replacement directory when a replace directive
// is in effect, and the repository root when run inside the framework
// repository itself. Copied module code therefore always matches the framework
// version the project compiles against, and no particular on-disk layout is
// required of the project.
func findFrameworkRoot() (string, error) {
	dir, err := frameworkModuleDirFromGoList()
	if err != nil {
		return "", err
	}
	if dir == "" {
		// The framework is in the module graph but its source has not been
		// downloaded into the module cache yet; fetch it and resolve again.
		if err = downloadFrameworkModule(); err != nil {
			return "", err
		}
		if dir, err = frameworkModuleDirFromGoList(); err != nil {
			return "", err
		}
	}
	if dir == "" {
		return "", errors.Newf("framework source for %s not found; run `go mod download %s` and retry", frameworkModulePath, frameworkModulePath)
	}
	return dir, nil
}

func frameworkModuleDirFromGoList() (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", frameworkModulePath).Output()
	if err != nil {
		return "", errors.Wrapf(goCommandError(err), "resolving framework source: the project must depend on %s", frameworkModulePath)
	}
	return strings.TrimSpace(string(out)), nil
}

func downloadFrameworkModule() error {
	out, err := exec.Command("go", "mod", "download", frameworkModulePath).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "downloading framework module %s: %s", frameworkModulePath, strings.TrimSpace(string(out)))
	}
	return nil
}

// goCommandError surfaces the go command's stderr, which carries the actual
// diagnosis (unknown dependency, missing go.mod, ...), instead of the bare
// "exit status 1".
func goCommandError(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return errors.Newf("%s", strings.TrimSpace(string(exitErr.Stderr)))
	}
	return err
}

func readProjectModulePath() (string, error) {
	content, err := os.ReadFile("go.mod")
	if err != nil {
		return "", fmt.Errorf("failed to read go.mod: %w", err)
	}

	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}

	return "", errors.New("module name not found in go.mod")
}
