package cmd

import (
	"fmt"
	"os"

	"github.com/ncr-devops-platform/nagiosfoundation/lib/app/nagiosfoundation"
	nf "github.com/ncr-devops-platform/nagiosfoundation/lib/app/nagiosfoundation/check_service"
	"github.com/spf13/cobra"
)

var state, user, manager string

// Execute runs the root command
func Execute() {
	var name string

	var rootCmd = &cobra.Command{
		Use:   "check_service",
		Short: "Determine if a service is running.",
		Long: `Perform various checks for a service. These checks depend on the options
given and the --name (-n) option is always required.` + getHelpOsConstrained(),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.ParseFlags(os.Args)

			nf.CheckService(name, state, user, manager)
		},
	}

	nagiosfoundation.AddVersionCommand(rootCmd)

	const nameFlag = "name"
	rootCmd.Flags().StringVarP(&name, nameFlag, "n", "", "service name")
	rootCmd.MarkFlagRequired(nameFlag)

	addFlagsOsConstrained(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}