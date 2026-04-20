package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/lorenjphillips/sv/internal/backup"
	"github.com/lorenjphillips/sv/internal/config"
)

func init() {
	rootCmd.AddCommand(syncCmd)
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Run a backup now",
	RunE:  runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("no config found: %w", err)
	}

	start := time.Now()
	fmt.Println(titleStyle.Render("sv sync"))

	if err := backup.Run(cfg); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(successStyle.Render(fmt.Sprintf("Done in %s", time.Since(start).Round(time.Second))))
	return nil
}
