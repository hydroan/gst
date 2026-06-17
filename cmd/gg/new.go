package main

import (
	pkgnew "github.com/hydroan/gst/internal/codegen/new"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "new a project",
	Long:  "new a project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectName := args[0]
		checkErr(pkgnew.Run(projectName))
	},
}
