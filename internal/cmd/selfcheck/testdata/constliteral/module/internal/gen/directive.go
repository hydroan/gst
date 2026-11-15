package gen

import (
	"os/exec"

	"example.com/module/internal/ggconst"
)

// directives holds a string a directive speaks for, a directive that gives
// no reason, and two that speak for no string the check reports: on a line
// where no string spells a constant, and on one whose string is handed to
// os/exec.
func directives(name string) []any {
	return []any{
		name == "Run",               //gst:literal the Run of another package
		name == "model",             //gst:literal
		name == "other",             //gst:literal nothing on the line spells a constant
		exec.Command("git", "init"), //gst:literal the subcommand of git
		ggconst.BootstrapRun,
	}
}
