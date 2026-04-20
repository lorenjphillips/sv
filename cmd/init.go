package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/lorenjphillips/skill-vault/internal/config"
	"github.com/lorenjphillips/skill-vault/internal/detect"
	"github.com/lorenjphillips/skill-vault/internal/schedule"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			MarginBottom(1)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

const banner = `
     _    _ _ _                       _ _
 ___| | _(_) | |    __   ____ _ _   _| | |_
/ __| |/ / | | |____\ \ / / _` + "`" + ` | | | | | __|
\__ \   <| | | |_____\ V / (_| | |_| | | |_
|___/_|\_\_|_|_|      \_/ \__,_|\__,_|_|\__|
`

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive setup — detect tools, choose backups, configure schedule",
	RunE:  runInit,
}

func cancelled() {
	fmt.Println()
	fmt.Println(dimStyle.Render("Setup cancelled."))
	os.Exit(0)
}

func checkCancel(err error) error {
	if err != nil && errors.Is(err, huh.ErrUserAborted) {
		cancelled()
	}
	return err
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println(titleStyle.Render(banner))
	fmt.Println(dimStyle.Render("Back up your AI agent skills, config, and conversations."))
	fmt.Println()

	if config.Exists() {
		var overwrite bool
		if err := checkCancel(huh.NewConfirm().
			Title("Existing config found. Overwrite?").
			Value(&overwrite).
			Run()); err != nil {
			return err
		}
		if !overwrite {
			fmt.Println("Keeping existing config.")
			return nil
		}
	}

	fmt.Println(dimStyle.Render("Scanning for AI tools..."))
	tools := detect.Scan()

	if len(tools) == 0 {
		fmt.Println("No AI tools detected. Nothing to back up.")
		return nil
	}

	fmt.Println()
	for _, t := range tools {
		fmt.Printf("  %s %s %s\n",
			successStyle.Render("found"),
			t.Description,
			dimStyle.Render(fmt.Sprintf("(%s)", detect.FormatSize(t.DiskSize))))
	}
	fmt.Println()

	selectedTools, err := selectTools(tools)
	if err != nil {
		return err
	}
	if len(selectedTools) == 0 {
		fmt.Println("No tools selected.")
		return nil
	}

	toolConfigs, err := selectCategories(tools, selectedTools)
	if err != nil {
		return err
	}

	backupTargets, err := selectBackupTargets(toolConfigs)
	if err != nil {
		return err
	}

	cfg := &config.Config{
		Tools: toolConfigs,
	}

	for _, target := range backupTargets {
		switch target {
		case "git":
			gitCfg, err := configureGit()
			if err != nil {
				return err
			}
			cfg.Git = gitCfg
		case "s3":
			s3Cfg, err := configureS3()
			if err != nil {
				return err
			}
			cfg.S3 = s3Cfg
		case "gcs":
			gcsCfg, err := configureGCS()
			if err != nil {
				return err
			}
			cfg.GCS = gcsCfg
		case "azure":
			azureCfg, err := configureAzure()
			if err != nil {
				return err
			}
			cfg.Azure = azureCfg
		case "icloud":
			cfg.ICloud = config.ICloudConfig{Enabled: true}
		case "timemachine":
			cfg.TimeMachine = config.TimeMachineConfig{Enabled: true}
		}
	}

	scheduleCfg, err := configureSchedule()
	if err != nil {
		return err
	}
	cfg.Schedule = scheduleCfg

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println()
	fmt.Println(successStyle.Render("Config saved to " + config.Path()))

	if scheduleCfg.Enabled {
		if err := schedule.Install(scheduleCfg.Interval); err != nil {
			return fmt.Errorf("installing schedule: %w", err)
		}
		fmt.Println(successStyle.Render("Scheduled sync installed (launchd)"))
	}

	fmt.Println()
	fmt.Println(dimStyle.Render("Run 'skill-vault sync' to back up now."))
	return nil
}

func selectTools(tools []detect.Tool) ([]string, error) {
	options := make([]huh.Option[string], len(tools))
	for i, t := range tools {
		label := fmt.Sprintf("%s (%s)", t.Description, detect.FormatSize(t.DiskSize))
		options[i] = huh.NewOption(label, t.Name).Selected(true)
	}

	var selected []string
	err := checkCancel(huh.NewMultiSelect[string]().
		Title("Which tools do you want to back up?").
		Description("Space to toggle, Enter to confirm").
		Options(options...).
		Value(&selected).
		Run())

	return selected, err
}

func selectCategories(tools []detect.Tool, selectedNames []string) (map[string]config.ToolConfig, error) {
	result := make(map[string]config.ToolConfig)
	selectedSet := make(map[string]bool)
	for _, n := range selectedNames {
		selectedSet[n] = true
	}

	for _, t := range tools {
		if !selectedSet[t.Name] {
			continue
		}

		categories := make(map[string]bool)
		for _, p := range t.Paths {
			categories[string(p.Category)] = true
		}

		if len(categories) == 0 {
			continue
		}

		if len(categories) == 1 {
			cats := make([]string, 0, len(categories))
			for c := range categories {
				cats = append(cats, c)
			}
			result[t.Name] = config.ToolConfig{Enabled: true, Categories: cats}
			continue
		}

		options := make([]huh.Option[string], 0, len(categories))
		for c := range categories {
			options = append(options, huh.NewOption(c, c).Selected(true))
		}

		var selected []string
		err := checkCancel(huh.NewMultiSelect[string]().
			Title(fmt.Sprintf("What to back up from %s?", t.Description)).
			Description("Space to toggle, Enter to confirm").
			Options(options...).
			Value(&selected).
			Run())
		if err != nil {
			return nil, err
		}

		result[t.Name] = config.ToolConfig{Enabled: true, Categories: selected}
	}

	return result, nil
}

func selectBackupTargets(toolConfigs map[string]config.ToolConfig) ([]string, error) {
	hasConversations := false
	for _, t := range toolConfigs {
		for _, c := range t.Categories {
			if c == "conversations" {
				hasConversations = true
			}
		}
	}

	options := []huh.Option[string]{
		huh.NewOption("Git repository (GitHub, GitLab, etc.)", "git"),
	}

	cloudHint := "Compressed daily snapshots for conversation logs"
	if hasConversations {
		cloudHint = "Recommended — conversation logs are too large for git"
	}

	options = append(options,
		huh.NewOption("AWS S3", "s3"),
		huh.NewOption("Google Cloud Storage", "gcs"),
		huh.NewOption("Azure Blob Storage", "azure"),
		huh.NewOption("iCloud Drive", "icloud"),
		huh.NewOption("Time Machine (verify inclusion)", "timemachine"),
	)

	var selected []string
	err := checkCancel(huh.NewMultiSelect[string]().
		Title("Where should skill-vault back up to?").
		Description(cloudHint).
		Options(options...).
		Value(&selected).
		Run())

	return selected, err
}

func configureGit() (config.GitConfig, error) {
	var cfg config.GitConfig
	cfg.Enabled = true

	var provider string
	err := checkCancel(huh.NewSelect[string]().
		Title("Git provider").
		Options(
			huh.NewOption("GitHub", "github"),
			huh.NewOption("GitLab", "gitlab"),
			huh.NewOption("Other", "other"),
		).
		Value(&provider).
		Run())
	if err != nil {
		return cfg, err
	}
	cfg.Provider = provider

	placeholder := "git@github.com:you/ai-backup.git"
	if provider == "gitlab" {
		placeholder = "git@gitlab.com:you/ai-backup.git"
	} else if provider == "other" {
		placeholder = "git@git.example.com:you/ai-backup.git"
	}

	home, _ := os.UserHomeDir()
	err = checkCancel(huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Repository URL").
				Placeholder(placeholder).
				Value(&cfg.Repo),
			huh.NewInput().
				Title("Local clone path").
				Placeholder(home+"/Development/ai-backup").
				Value(&cfg.LocalPath),
		),
	).Run())

	if err != nil {
		return cfg, err
	}

	if cfg.Repo == "" {
		return cfg, fmt.Errorf("repository URL is required")
	}

	if cfg.LocalPath == "" {
		cfg.LocalPath = home + "/Development/ai-backup"
	}

	return cfg, nil
}

func configureS3() (config.S3Config, error) {
	var cfg config.S3Config
	cfg.Enabled = true
	cfg.Region = "us-east-1"

	err := checkCancel(huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("S3 bucket name").
				Placeholder("my-ai-backups").
				Value(&cfg.Bucket),
			huh.NewInput().
				Title("AWS CLI profile").
				Description("Leave blank for default profile").
				Placeholder("default").
				Value(&cfg.Profile),
			huh.NewInput().
				Title("AWS region").
				Value(&cfg.Region),
		),
	).Run())

	return cfg, err
}

func configureGCS() (config.GCSConfig, error) {
	var cfg config.GCSConfig
	cfg.Enabled = true

	err := checkCancel(huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("GCS bucket name").
				Placeholder("my-ai-backups").
				Value(&cfg.Bucket),
			huh.NewInput().
				Title("GCP project ID").
				Description("Leave blank to use gcloud default project").
				Placeholder("my-project").
				Value(&cfg.Project),
		),
	).Run())

	return cfg, err
}

func configureAzure() (config.AzureConfig, error) {
	var cfg config.AzureConfig
	cfg.Enabled = true

	err := checkCancel(huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Blob container name").
				Placeholder("ai-backups").
				Value(&cfg.Container),
			huh.NewInput().
				Title("Storage account name").
				Value(&cfg.StorageAcct),
		),
	).Run())

	return cfg, err
}

func configureSchedule() (config.ScheduleConfig, error) {
	var cfg config.ScheduleConfig

	err := checkCancel(huh.NewConfirm().
		Title("Set up automatic backups?").
		Description("Creates a macOS launchd job to sync on a schedule").
		Value(&cfg.Enabled).
		Run())
	if err != nil || !cfg.Enabled {
		return cfg, err
	}

	var intervalChoice string
	err = checkCancel(huh.NewSelect[string]().
		Title("How often should skill-vault sync?").
		Options(
			huh.NewOption("Every 6 hours", "6h"),
			huh.NewOption("Every 12 hours", "12h"),
			huh.NewOption("Every 24 hours (daily)", "24h"),
			huh.NewOption("Every 2 days", "48h"),
			huh.NewOption("Every 7 days (weekly)", "168h"),
			huh.NewOption("Custom", "custom"),
		).
		Value(&intervalChoice).
		Run())
	if err != nil {
		return cfg, err
	}

	if intervalChoice == "custom" {
		err = checkCancel(huh.NewInput().
			Title("Custom interval").
			Description("Go duration format: e.g. 8h, 72h (minimum 1h)").
			Placeholder("24h").
			Value(&intervalChoice).
			Run())
		if err != nil {
			return cfg, err
		}
	}

	cfg.Interval = strings.TrimSpace(intervalChoice)
	return cfg, nil
}
