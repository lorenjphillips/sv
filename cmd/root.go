package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "sv",
	Short: "Back up your AI agent skills, config, and conversation logs",
	Long: `sv detects installed AI coding tools and backs up their skills,
configs, memory, and conversation logs.

Supports 17 tools including Claude Code, Cursor, Codex, Windsurf, Aider,
Continue, Copilot, Amp, Cline, Roo Code, and more.

Backup targets: Git (GitHub/GitLab), AWS S3, Google Cloud Storage,
Azure Blob Storage, iCloud Drive, and Time Machine.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
