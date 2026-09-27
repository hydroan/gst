package gghelper

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gofrs/flock"
)

// PinnedCommand builds the main package pkg of the module named by module at
// version and returns the command that runs the program in the working
// directory and the environment of the caller, with args as the program's
// own arguments: how gg gen runs the protobuf plugins and gg lint runs
// golangci-lint. The program is built apart from the project's module, so
// nothing is installed and the project's go.mod names nothing for it: under
// the user cache directory a go.mod of gg's own requires the module at that
// version and nothing else (see pinnedModFile), and go build, run in that
// directory with -mod=mod so that it fills the go.sum in beside it and with
// GOWORK=off so that no workspace above it chooses the versions, writes the
// program there too. The go directive of that go.mod names the go command
// at hand, so the build runs on it rather than fetching a toolchain. The
// module is downloaded on the first build and read from the module cache
// from then on, network or not, and a later build finds the program up to
// date and leaves it, so a run costs about what go run costs with its
// program cached. go run itself is not used because the program would
// inherit the build's environment: golangci-lint runs go list in the
// project, which with GOWORK=off would read the project without its go.work
// and with -mod=mod in GOFLAGS could rewrite the project's go.mod.
func PinnedCommand(module, version, pkg string, args ...string) (*exec.Cmd, error) {
	toolchain, err := goToolchainVersion()
	if err != nil {
		return nil, err
	}
	dir, err := pinnedModuleDir(module, version)
	if err != nil {
		return nil, err
	}
	// gg processes running at once (the cmd/gg tests do) build the same
	// program in turn: the lock holds the go.mod, the go.sum and the program
	// still while one of them writes.
	lock := flock.New(filepath.Join(dir, "build.lock"))
	if err = lock.Lock(); err != nil {
		return nil, errors.Wrapf(err, "lock the module directory of %s@%s", module, version)
	}
	defer lock.Unlock()
	if err = writePinnedModFile(dir, module, version, toolchain); err != nil {
		return nil, err
	}
	program := filepath.Join(dir, path.Base(pkg))
	// #nosec G204 -- the arguments are literal flags, a path under the module
	// directory gg itself wrote in the user cache directory, and the package
	// the callers spell out in constants.
	build := exec.Command("go", "build", "-mod=mod", "-o", program, pkg)
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off")
	build.Stderr = os.Stderr
	if err = build.Run(); err != nil {
		return nil, errors.Wrapf(err, "build %s@%s", pkg, version)
	}
	// #nosec G204 -- the program was just built under the module directory.
	return exec.Command(program, args...), nil
}

// userCacheDir is os.UserCacheDir, replaced by the tests of this package to
// keep their modules out of the user's cache.
var userCacheDir = os.UserCacheDir

// goToolchainVersion returns the version of the go command PinnedCommand
// builds with, go1.27.1, as go env GOVERSION reports it.
func goToolchainVersion() (string, error) {
	out, err := exec.Command("go", "env", "GOVERSION").Output()
	if err != nil {
		return "", errors.Wrap(err, "read the version of the go command")
	}
	return strings.TrimSpace(string(out)), nil
}

// pinnedModuleDir returns the directory PinnedCommand builds in for module
// at version, one per module and version under the user cache directory,
// so the go.sum the go command writes there on the first build, and the
// program, serve every later one.
func pinnedModuleDir(module, version string) (string, error) {
	base, err := userCacheDir()
	if err != nil {
		return "", errors.Wrap(err, "resolve user cache directory")
	}
	dir := filepath.Join(base, "gg", "run", module+"@"+version)
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return "", errors.Wrapf(err, "create the module directory of %s@%s", module, version)
	}
	return dir, nil
}

// writePinnedModFile writes the go.mod of dir when it does not say what
// pinnedModFile says for module, version and toolchain, and leaves it alone
// otherwise.
func writePinnedModFile(dir, module, version, toolchain string) error {
	modFile := filepath.Join(dir, "go.mod")
	content := pinnedModFile(module, version, toolchain)
	existing, err := os.ReadFile(modFile)
	if err == nil && string(existing) == content {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "read %s", modFile)
	}
	if err = os.WriteFile(modFile, []byte(content), 0o600); err != nil {
		return errors.Wrapf(err, "write %s", modFile)
	}
	return nil
}

// goDirectiveVersion reads the go directive out of a toolchain version,
// 1.27.1 from go1.27.1; a development toolchain gives none.
var goDirectiveVersion = regexp.MustCompile(`^go(\d+\.\d+(?:\.\d+)?)`)

// pinnedModFile returns the go.mod of the module PinnedCommand builds in: a
// module of its own, the go directive of the go command building it, when
// its toolchain version names one, and the one requirement. For
// google.golang.org/protobuf at v1.36.12 under go1.27.1 it returns
//
//	module gg.run
//
//	go 1.27.1
//
//	require google.golang.org/protobuf v1.36.12
func pinnedModFile(module, version, toolchain string) string {
	var b strings.Builder
	b.WriteString("module gg.run\n\n")
	if m := goDirectiveVersion.FindStringSubmatch(toolchain); m != nil {
		fmt.Fprintf(&b, "go %s\n\n", m[1])
	}
	fmt.Fprintf(&b, "require %s %s\n", module, version)
	return b.String()
}
