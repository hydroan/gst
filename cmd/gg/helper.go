package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
)

func checkErr(err error) {
	if err == nil {
		return
	}
	panic(err)
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
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

func writeFileWithLog(filename string, content string) {
	checkErr(writeGeneratedFile(filename, content, true))
}

func writeGeneratedFile(filename string, content string, log bool) error {
	if fileExists(filename) {
		oldData, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		if string(oldData) == content {
			if log {
				clioutput.Item("SKIP", "%s", filename)
			}
		} else {
			if log {
				clioutput.Status(clioutput.StyleWarn, clioutput.SymbolSuccess, "UPDATE", "%s", filename)
			}
			if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
				return err
			}
		}
	} else {
		if log {
			clioutput.Success("CREATE", "%s", filename)
		}
		if err := ensureParentDir(filename); err != nil {
			return err
		}
		if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func getModuleName() (string, error) {
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
