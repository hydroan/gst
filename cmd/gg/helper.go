package main

import (
	"os"

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

func writeFileWithLog(filename string, content string) {
	checkErr(writeGeneratedFile(filename, content, true))
}

func writeGeneratedFile(filename string, content string, log bool) error {
	if gghelper.FileExists(filename) {
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
		if err := gghelper.EnsureParentDir(filename); err != nil {
			return err
		}
		if err := os.WriteFile(filename, []byte(content), ggconst.FileModeGenerated); err != nil {
			return err
		}
	}
	return nil
}
