package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lorenjphillips/sv/internal/config"
	"github.com/lorenjphillips/sv/internal/detect"
	"github.com/lorenjphillips/sv/internal/schedule"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show backup configuration and health",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	fmt.Println(titleStyle.Render("sv status"))

	cfg, err := config.Load()
	if err != nil {
		fmt.Println(dimStyle.Render("Not configured — run 'sv init'"))
		fmt.Println()
		fmt.Println("Detected tools:")
		for _, t := range detect.Scan() {
			fmt.Printf("  %s %s %s\n",
				successStyle.Render("found"),
				t.Description,
				dimStyle.Render(detect.FormatSize(t.DiskSize)))
		}
		return nil
	}

	fmt.Println("Tools:")
	for name, t := range cfg.Tools {
		if t.Enabled {
			fmt.Printf("  %s %s  %v\n", successStyle.Render("on"), name, t.Categories)
		}
	}

	fmt.Println()
	fmt.Println("Backup targets:")
	printTarget := func(enabled bool, label, detail string) {
		if enabled {
			fmt.Printf("  %s %s: %s\n", successStyle.Render("on"), label, detail)
		} else {
			fmt.Printf("  %s %s\n", dimStyle.Render("off"), label)
		}
	}

	printTarget(cfg.Git.Enabled, "Git ("+cfg.Git.Provider+")", cfg.Git.Repo)
	printTarget(cfg.S3.Enabled, "AWS S3", cfg.S3.Bucket)
	printTarget(cfg.GCS.Enabled, "Google Cloud Storage", cfg.GCS.Bucket)
	printTarget(cfg.Azure.Enabled, "Azure Blob", cfg.Azure.Container)
	printTarget(cfg.ICloud.Enabled, "iCloud Drive", "~/Library/Mobile Documents/.../sv/")
	printTarget(cfg.TimeMachine.Enabled, "Time Machine", "verify inclusion")

	fmt.Println()
	fmt.Println("Schedule:")
	fmt.Printf("  Status: %s\n", schedule.Status())
	fmt.Printf("  Last sync: %s\n", schedule.LastRun())
	if cfg.Schedule.Enabled {
		fmt.Printf("  Interval: %s\n", cfg.Schedule.Interval)
	}

	return nil
}
