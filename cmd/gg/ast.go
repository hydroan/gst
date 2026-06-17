package main

import (
	"fmt"

	codegenast "github.com/hydroan/gst/internal/codegen/ast"
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
		f, dump, err := codegenast.Dump(args[0], nil)
		checkErr(err)
		if ispretty {
			pretty.Println(f)
		} else {
			fmt.Println(dump)
		}
	},
}

var astgo2jsonCmd = &cobra.Command{
	Use:   "go2json",
	Short: "convert go file to json",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
	},
}

var astjson2goCmd = &cobra.Command{
	Use:   "json2go",
	Short: "convert json to go file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
	},
}

func init() {
	astdumpCmd.Flags().BoolVarP(&ispretty, "pretty", "p", false, "enable pretty print")

	astCmd.AddCommand(astdumpCmd, astgo2jsonCmd, astjson2goCmd)
}
