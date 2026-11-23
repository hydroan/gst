package main

import (
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// The codes gg exits with besides 0: exitFailure for a command that failed,
// exitUsage for a command line gg cannot run.
const (
	exitFailure = 1
	exitUsage   = 2
)

// errReported is the error of a command whose failure is on the terminal
// already, printed by the program it ran: run adds no line to it.
var errReported = errors.New("the failure is reported")

// run runs gg with args and returns the code gg exits with. A command that
// fails has its error printed once, as the last line of the output. A
// command line gg cannot run — a command or a flag gg does not have,
// arguments the command does not take — is refused by cobra before the
// command starts (see startCommand): its error is followed by where the
// usage is, and the code tells it apart from a command that ran and failed.
func run(args []string) int {
	commandStarted = false
	rootCmd.SetArgs(args)
	cmd, err := rootCmd.ExecuteC()
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errReported):
		return exitFailure
	case !commandStarted:
		// The first line says what gg cannot run; cobra's suggestion of the
		// commands meant, when it has one, follows the pointer to the usage.
		message, suggestion, suggested := strings.Cut(strings.TrimRight(err.Error(), "\n"), "\n")
		if suggested {
			suggestion = "\n" + suggestion
		}
		clioutput.Fail("%s\n  Run '%s --help' for usage.%s", message, cmd.CommandPath(), suggestion)
		return exitUsage
	default:
		clioutput.Fail("%v", err)
		return exitFailure
	}
}
