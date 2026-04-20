package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lorenjphillips/sv/internal/schedule"
)

func init() {
	rootCmd.AddCommand(uninstallCmd)
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the launchd scheduled sync job",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := schedule.Uninstall(); err != nil {
			return err
		}
		fmt.Println(successStyle.Render("Launchd job removed"))
		return nil
	},
}
