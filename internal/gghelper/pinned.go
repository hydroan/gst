package gghelper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/cockroachdb/errors"
)

// PinnedCommand returns the command that runs the main package pkg, of the
// module named by module, at version through go run, in the working
// directory of the caller, with args as the program's own arguments: how gg
// gen runs the
// protobuf plugins and gg lint runs golangci-lint, built apart from the
// project's module and cached, so nothing is installed and the project's
// go.mod names nothing for them. The run is module-aware without being the
// project's: go run reads, through -modfile, a go.mod of gg's own under the
// user cache directory, which requires the module at that version and
// nothing else (see pinnedModFile), with -mod=mod so the go command may
// fill the go.sum beside it in. The module is downloaded on the first run
// and read from the module cache from then on, network or not, and the
// build cache keeps later runs to the time of starting the program. go run
// pkg@version would instead ask the module proxy about the module's
// deprecation on every run and fail without a network even with the module
// cached.
func PinnedCommand(module, version, pkg string, args ...string) (*exec.Cmd, error) {
	modFile, err := pinnedModFilePath(module, version)
	if err != nil {
		return nil, err
	}
	// #nosec G204 -- the arguments are literal flags, a module file gg itself
	// wrote under the user cache directory, and the package and arguments
	// the callers spell out in constants.
	cmd := exec.Command("go", append([]string{"run", "-modfile", modFile, pkg}, args...)...)
	cmd.Env = append(os.Environ(),
		"GOFLAGS="+strings.TrimSpace(os.Getenv("GOFLAGS")+" -mod=mod"),
		"GOWORK=off")
	return cmd, nil
}

// userCacheDir is os.UserCacheDir, replaced by the tests of this package to
// keep their modules out of the user's cache.
var userCacheDir = os.UserCacheDir

// pinnedModFilePath returns the path of the go.mod PinnedCommand reads for
// module at version, writing it on the first call: one directory per module
// and version under the user cache directory, so the go.sum the go command
// writes beside it on the first run serves every later one.
func pinnedModFilePath(module, version string) (string, error) {
	base, err := userCacheDir()
	if err != nil {
		return "", errors.Wrap(err, "resolve user cache directory")
	}
	dir := filepath.Join(base, "gg", "run", module+"@"+version)
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return "", errors.Wrapf(err, "create the module directory of %s@%s", module, version)
	}
	modFile := filepath.Join(dir, "go.mod")
	content := pinnedModFile(module, version, runtime.Version())
	existing, err := os.ReadFile(modFile)
	if err == nil && string(existing) == content {
		return modFile, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", errors.Wrapf(err, "read %s", modFile)
	}
	if err = os.WriteFile(modFile, []byte(content), 0o600); err != nil {
		return "", errors.Wrapf(err, "write %s", modFile)
	}
	return modFile, nil
}

// goDirectiveVersion reads the go directive out of a toolchain version,
// 1.27.1 from go1.27.1; a development toolchain gives none.
var goDirectiveVersion = regexp.MustCompile(`^go(\d+\.\d+(?:\.\d+)?)`)

// pinnedModFile returns the go.mod of the module PinnedCommand runs in: a
// module of its own, the go directive of the toolchain running it, when the
// toolchain version names one, and the one requirement. For
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
