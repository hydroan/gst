package main

import (
	"fmt"
	"os"

	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/goast"
	"github.com/kr/pretty"
	"github.com/spf13/cobra"
)

var ispretty bool

var astCmd = &cobra.Command{
	Use:   "ast",
	Short: "golang ast utility",
}

var astdumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "dump ast info",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		f, dump, err := goast.Dump(args[0], nil)
		if err != nil {
			clioutput.Error("", "%v", err)
			os.Exit(1)
		}
		if ispretty {
			pretty.Println(f)
		} else {
			fmt.Println(dump)
		}
	},
}

func init() {
	astdumpCmd.Flags().BoolVarP(&ispretty, "pretty", "p", false, "enable pretty print")

	astCmd.AddCommand(astdumpCmd)
}
