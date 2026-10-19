package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// recordFile is the file in the record directory listing the test binaries
// linked: a line "<import path>\t<build ID>" per binary.
const recordFile = "links"

// recordLink runs tool with args the way go would have, and, when the tool
// linked a test binary, records the package it tests with the binary's build
// ID. It returns the tool's exit code: go reads the tool's success off it, so
// a failure to record is reported on stderr and changes nothing.
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

// recordTestLink appends the test binary the link invocation args produced
// to the record, and does nothing for a link of anything else. The tested
// package is read off the link's import config, which lists the test main
// package as "<import path>.test".
//
// go writes the binary's content ID into its build ID only after the linker
// returns, in place, so the wrapper computes it itself the way go does, see
// contentID, from the build ID go gave the linker: the finalized build ID is
// that one with its last part replaced by the content ID.
func recordTestLink(recordDir string, args []string) error {
	output, importcfg, buildID := flagValue(args, "-o"), flagValue(args, "-importcfg"), flagValue(args, "-buildid")
	if !strings.HasSuffix(output, ".test") || importcfg == "" || buildID == "" {
		return nil
	}
	importPath, err := testedPackage(importcfg)
	if err != nil {
		return err
	}
	content, err := contentID(output, buildID)
	if err != nil {
		return err
	}
	buildID = buildID[:strings.LastIndex(buildID, "/")+1] + content
	// #nosec G703 -- the record directory is the one make created for this run.
	f, err := os.OpenFile(filepath.Join(recordDir, recordFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.WithStack(err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\t%s\n", importPath, buildID)
	return errors.WithStack(err)
}

// flagValue returns the value of the flag name in args, "" when the flag is
// absent: go passes the linker a flag's value as the next argument, or joined
// to the flag with "=".
func flagValue(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
		if value, ok := strings.CutPrefix(arg, name+"="); ok {
			return value
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
