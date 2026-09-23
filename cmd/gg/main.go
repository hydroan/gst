package main

import (
	"os"

	"github.com/hydroan/gst/internal/clioutput"
)

func main() {
	// A command that fails lands here: its error is printed once, the way
	// every gg command reports one, since the root command keeps cobra quiet.
	if err := rootCmd.Execute(); err != nil {
		clioutput.Error("", "%v", err)
		os.Exit(1)
	}
}
