package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// recordFile is the file in the record directory listing the test binaries
// kept there: a line "<import path>\t<file name>" per binary.
const recordFile = "links"

// recordLink runs tool with args the way go would have, and, when the tool
// linked a test binary, keeps the binary in the record directory with the
// package it tests. It returns the tool's exit code: go reads the tool's
// success off it, so a failure to record is reported on stderr and changes
// nothing.
//
// The binary is kept as a hard link to the one go linked, in the work
// directory go removes when the run ends. go writes the binary's content ID
// into its build ID only after the linker returns, in place, so the link
// carries the final build ID once the run is over: the wrapper cannot read
// it, and writeback does.
func recordLink(recordDir, tool string, args []string) int {
	// #nosec G702 -- go names the tool and its arguments; this is its -toolexec wrapper.
	cmd := exec.Command(tool, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		log.Print(err)
		return 1
	}
	if filepath.Base(tool) != "link" {
		return 0
	}
	if err := recordTestLink(recordDir, args); err != nil {
		log.Printf("recording the test link: %v", err)
	}
	return 0
}

// recordTestLink keeps the test binary the link invocation args produced
// and appends it to the record, and does nothing for a link of anything
// else. The tested package is read off the link's import config, which
// lists the test main package as "<import path>.test".
func recordTestLink(recordDir string, args []string) error {
	output, importcfg := flagValue(args, "-o"), flagValue(args, "-importcfg")
	if !strings.HasSuffix(output, ".test") || importcfg == "" {
		return nil
	}
	importPath, err := testedPackage(importcfg)
	if err != nil {
		return err
	}
	// The wrapper runs once per link, so its process ID tells binaries of
	// one name apart.
	name := fmt.Sprintf("%d-%s", os.Getpid(), filepath.Base(output))
	if err = keepBinary(output, filepath.Join(recordDir, name)); err != nil {
		return err
	}
	// #nosec G703 -- the record directory is the one make created for this run.
	f, err := os.OpenFile(filepath.Join(recordDir, recordFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.WithStack(err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\t%s\n", importPath, name)
	return errors.WithStack(err)
}

// keepBinary links the binary at output to path, and copies it when the two
// are on different file systems, where go's later rewrite of the build ID
// does not reach the copy: writeback then reads a build ID go never
// finalized and reports the binary as unknown, which is the honest outcome.
func keepBinary(output, path string) error {
	if err := os.Link(output, path); err == nil {
		return nil
	}
	// #nosec G703 -- output is the linker's own -o under go's work directory.
	src, err := os.Open(output)
	if err != nil {
		return errors.WithStack(err)
	}
	defer src.Close()
	// #nosec G703 -- path is under the record directory make created for this run.
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err = io.Copy(dst, src); err != nil {
		dst.Close()
		return errors.WithStack(err)
	}
	return errors.WithStack(dst.Close())
}

// flagValue returns the value following the flag name in args, "" when the
// flag is absent: go passes the linker its flags as separate arguments.
func flagValue(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// testedPackage returns the import path of the package under test named by
// the link's import config: the line "packagefile <path>.test=<file>" names
// the test main package, and there is one.
func testedPackage(importcfg string) (string, error) {
	// #nosec G703 -- importcfg is the linker's own -importcfg under go's work directory.
	f, err := os.Open(importcfg)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		rest, ok := strings.CutPrefix(scanner.Text(), "packagefile ")
		if !ok {
			continue
		}
		path, _, _ := strings.Cut(rest, "=")
		if tested, ok := strings.CutSuffix(path, ".test"); ok {
			return tested, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", errors.WithStack(err)
	}
	return "", errors.Newf("%s names no test main package", importcfg)
}
