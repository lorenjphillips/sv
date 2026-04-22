package cmd

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/lorenjphillips/sv/internal/config"
	"github.com/lorenjphillips/sv/internal/detect"
	"github.com/lorenjphillips/sv/internal/schedule"
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

var bannerLines = [3]string{
	"╔═╗╦╔═╦╦  ╦    ╦  ╦╔═╗╦ ╦╦  ╔╦╗",
	"╚═╗╠╩╗║║  ║    ╚╗╔╝╠═╣║ ║║   ║ ",
	"╚═╝╩ ╩╩╩═╝╩═╝   ╚╝ ╩ ╩╚═╝╩═╝ ╩ ",
}

var bannerGradient = [3]lipgloss.Style{
	lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5FFFFF")),
	lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#AF87FF")),
	lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5FD7")),
}

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
	fmt.Println()
	for i, line := range bannerLines {
		fmt.Println(bannerGradient[i].Render("  " + line))
	}
	fmt.Println()
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

	// --- State variables bound across the wizard ---

	var selectedTools []string

	// Per-tool category selections.
	type toolCats struct {
		name        string
		description string
		options     []huh.Option[string]
		selected    []string
		autoSelect  bool
	}

	var perTool []toolCats
	for _, t := range tools {
		cats := make(map[string]bool)
		for _, p := range t.Paths {
			cats[string(p.Category)] = true
		}
		if len(cats) == 0 {
			continue
		}

		tc := toolCats{name: t.Name, description: t.Description}

		if len(cats) == 1 {
			for c := range cats {
				tc.selected = []string{c}
			}
			tc.autoSelect = true
		} else {
			for c := range cats {
				tc.options = append(tc.options, huh.NewOption(c, c).Selected(true))
			}
		}
		perTool = append(perTool, tc)
	}

	// --- Git target state ---
	var enableGit bool
	var gitProvider string
	var gitRepo string
	var gitLocalPath string

	// --- Cloud conversation target state ---
	var cloudTargets []string

	// --- S3 config state ---
	var s3Bucket string
	var s3Profile string
	var s3Region = "us-east-1"

	// --- GCS config state ---
	var gcsBucket string
	var gcsProject string

	// --- Azure config state ---
	var azureContainer string
	var azureStorageAcct string

	// --- Time Machine state ---
	var enableTimeMachine bool

	// --- Schedule state ---
	var schedEnabled bool
	var intervalChoice string

	// --- Helper: do any selected tools have conversations? ---
	hasConversations := func() bool {
		for i := range perTool {
			if !slices.Contains(selectedTools, perTool[i].name) {
				continue
			}
			for _, c := range perTool[i].selected {
				if c == "conversations" {
					return true
				}
			}
		}
		return false
	}

	// --- Build form groups ---

	home, _ := os.UserHomeDir()

	toolOptions := make([]huh.Option[string], len(tools))
	for i, t := range tools {
		label := fmt.Sprintf("%s (%s)", t.Description, detect.FormatSize(t.DiskSize))
		toolOptions[i] = huh.NewOption(label, t.Name).Selected(true)
	}

	groups := []*huh.Group{
		// Step 1: Select tools
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which tools do you want to back up?").
				Description("Space to toggle, Enter to confirm. Shift+Tab to go back.").
				Options(toolOptions...).
				Value(&selectedTools),
		),
	}

	// Step 2: Per-tool category selection
	for i := range perTool {
		tc := &perTool[i]
		if tc.autoSelect {
			continue
		}
		groups = append(groups, huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(fmt.Sprintf("What to back up from %s?", tc.description)).
				Description("Space to toggle, Enter to confirm").
				Options(tc.options...).
				Value(&tc.selected),
		).WithHideFunc(func() bool {
			return !slices.Contains(selectedTools, tc.name)
		}))
	}

	// Step 3: Git -- version control for skills, config, memory, and rules
	groups = append(groups, huh.NewGroup(
		huh.NewConfirm().
			Title("Version-control skills, config, memory, and rules with Git?").
			Description("Keeps a full history of changes in a Git repository").
			Value(&enableGit),
	))

	// Step 4: Git provider
	groups = append(groups, huh.NewGroup(
		huh.NewSelect[string]().
			Title("Git provider").
			Options(
				huh.NewOption("GitHub", "github"),
				huh.NewOption("GitLab", "gitlab"),
				huh.NewOption("Other", "other"),
			).
			Value(&gitProvider),
	).WithHideFunc(func() bool {
		return !enableGit
	}))

	// Step 5: Git repo + local path
	groups = append(groups, huh.NewGroup(
		huh.NewInput().
			Title("Repository URL").
			PlaceholderFunc(func() string {
				switch gitProvider {
				case "gitlab":
					return "git@gitlab.com:you/ai-backup.git"
				case "other":
					return "git@git.example.com:you/ai-backup.git"
				default:
					return "git@github.com:you/ai-backup.git"
				}
			}, &gitProvider).
			Value(&gitRepo),
		huh.NewInput().
			Title("Local clone path").
			Placeholder(home+"/Development/ai-backup").
			Value(&gitLocalPath),
	).WithHideFunc(func() bool {
		return !enableGit
	}))

	// Step 6: Cloud storage for conversations (only if conversations selected)
	groups = append(groups, huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Where should conversation logs be stored?").
			Description("Conversations are compressed into daily archives (too large for Git)").
			Options(
				huh.NewOption("AWS S3", "s3"),
				huh.NewOption("Google Cloud Storage", "gcs"),
				huh.NewOption("Azure Blob Storage", "azure"),
				huh.NewOption("iCloud Drive", "icloud"),
			).
			Value(&cloudTargets),
	).WithHideFunc(func() bool {
		return !hasConversations()
	}))

	// Step 7: S3 config
	groups = append(groups, huh.NewGroup(
		huh.NewInput().
			Title("S3 bucket name").
			Placeholder("my-ai-backups").
			Value(&s3Bucket),
		huh.NewInput().
			Title("AWS CLI profile").
			Description("Leave blank for default profile").
			Placeholder("default").
			Value(&s3Profile),
		huh.NewInput().
			Title("AWS region").
			Value(&s3Region),
	).WithHideFunc(func() bool {
		return !slices.Contains(cloudTargets, "s3")
	}))

	// Step 8: GCS config
	groups = append(groups, huh.NewGroup(
		huh.NewInput().
			Title("GCS bucket name").
			Placeholder("my-ai-backups").
			Value(&gcsBucket),
		huh.NewInput().
			Title("GCP project ID").
			Description("Leave blank to use gcloud default project").
			Placeholder("my-project").
			Value(&gcsProject),
	).WithHideFunc(func() bool {
		return !slices.Contains(cloudTargets, "gcs")
	}))

	// Step 9: Azure config
	groups = append(groups, huh.NewGroup(
		huh.NewInput().
			Title("Blob container name").
			Placeholder("ai-backups").
			Value(&azureContainer),
		huh.NewInput().
			Title("Storage account name").
			Value(&azureStorageAcct),
	).WithHideFunc(func() bool {
		return !slices.Contains(cloudTargets, "azure")
	}))

	// Step 10: Time Machine
	groups = append(groups, huh.NewGroup(
		huh.NewConfirm().
			Title("Verify tool directories are included in Time Machine?").
			Description("Checks that your AI tool directories are not excluded from backups").
			Value(&enableTimeMachine),
	))

	// Step 11: Schedule toggle
	groups = append(groups, huh.NewGroup(
		huh.NewConfirm().
			Title("Set up automatic backups?").
			Description("Creates a macOS launchd job to sync on a schedule").
			Value(&schedEnabled),
	))

	// Step 12: Schedule interval
	groups = append(groups, huh.NewGroup(
		huh.NewSelect[string]().
			Title("How often should sv sync?").
			Options(
				huh.NewOption("Every 6 hours", "6h"),
				huh.NewOption("Every 12 hours", "12h"),
				huh.NewOption("Every 24 hours (daily)", "24h"),
				huh.NewOption("Every 2 days", "48h"),
				huh.NewOption("Every 7 days (weekly)", "168h"),
			).
			Value(&intervalChoice),
	).WithHideFunc(func() bool {
		return !schedEnabled
	}))

	// --- Run the wizard as a single form (Shift+Tab navigates back) ---

	if err := checkCancel(huh.NewForm(groups...).Run()); err != nil {
		return err
	}

	// --- Build config from wizard state ---

	if len(selectedTools) == 0 {
		fmt.Println("No tools selected.")
		return nil
	}

	toolConfigs := make(map[string]config.ToolConfig)
	for i := range perTool {
		tc := &perTool[i]
		if !slices.Contains(selectedTools, tc.name) {
			continue
		}
		if len(tc.selected) == 0 {
			continue
		}
		toolConfigs[tc.name] = config.ToolConfig{Enabled: true, Categories: tc.selected}
	}

	cfg := &config.Config{
		Tools: toolConfigs,
	}

	if enableGit {
		if gitLocalPath == "" {
			gitLocalPath = home + "/Development/ai-backup"
		}
		cfg.Git = config.GitConfig{
			Enabled:   true,
			Provider:  gitProvider,
			Repo:      gitRepo,
			LocalPath: gitLocalPath,
		}
	}
	if slices.Contains(cloudTargets, "s3") {
		cfg.S3 = config.S3Config{
			Enabled: true,
			Bucket:  s3Bucket,
			Profile: s3Profile,
			Region:  s3Region,
		}
	}
	if slices.Contains(cloudTargets, "gcs") {
		cfg.GCS = config.GCSConfig{
			Enabled: true,
			Bucket:  gcsBucket,
			Project: gcsProject,
		}
	}
	if slices.Contains(cloudTargets, "azure") {
		cfg.Azure = config.AzureConfig{
			Enabled:     true,
			Container:   azureContainer,
			StorageAcct: azureStorageAcct,
		}
	}
	if slices.Contains(cloudTargets, "icloud") {
		cfg.ICloud = config.ICloudConfig{Enabled: true}
	}
	if enableTimeMachine {
		cfg.TimeMachine = config.TimeMachineConfig{Enabled: true}
	}

	if schedEnabled {
		cfg.Schedule = config.ScheduleConfig{
			Enabled:  true,
			Interval: strings.TrimSpace(intervalChoice),
		}
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println()
	fmt.Println(successStyle.Render("Config saved to " + config.Path()))

	if cfg.Schedule.Enabled {
		if err := schedule.Install(cfg.Schedule.Interval); err != nil {
			return fmt.Errorf("installing schedule: %w", err)
		}
		fmt.Println(successStyle.Render("Scheduled sync installed (launchd)"))
	}

	fmt.Println()
	fmt.Println(dimStyle.Render("Run 'sv sync' to back up now."))
	return nil
}
