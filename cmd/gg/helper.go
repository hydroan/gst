package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
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
			if err := os.WriteFile(filename, []byte(content), ggconst.FileModeGenerated); err != nil {
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
		if err := os.WriteFile(filename, []byte(content), ggconst.FileModeGenerated); err != nil {
			return err
		}
	}
	return nil
}

// currentProjectModulePath returns the module path of the project in the
// working directory, or "" when its go.mod cannot be read.
func currentProjectModulePath() string {
	modulePath, err := gghelper.ModulePath()
	if err != nil {
		return ""
	}
	return strings.Trim(modulePath, "/")
}

// relativePath returns filePath relative to the current working directory when possible.
func relativePath(filePath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filePath
	}
	relPath, err := filepath.Rel(cwd, filePath)
	if err != nil {
		return filePath
	}
	return relPath
}
