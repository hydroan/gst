package gen

import (
	"flag"
	"fmt"
	"os/exec"

	"example.com/module/internal/ggconst"
)

// localKind names a kind of the package's own, spelt like a constant of
// ggconst by chance.
const localKind = "model"

// accepted holds the strings the check leaves alone: handed to a package
// outside the module that is no sink, only containing the spelling of a
// constant, or named by a constant.
func accepted(name string) []any {
	return []any{
		exec.Command("git", "init"),
		fmt.Sprint("model"),
		&flag.Flag{Name: "model"},
		"the model is ready",
		"preinit" + name,
		localKind,
		ggconst.DirModel,
	}
}
