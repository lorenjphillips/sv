package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lorenjphillips/sv/internal/config"
	"github.com/lorenjphillips/sv/internal/detect"
)

// cmdTimeout is the maximum wall-clock time for any subprocess. This prevents
// indefinite hangs when running under launchd (e.g. a credential prompt that
// will never be answered).
const cmdTimeout = 5 * time.Minute

// Preflight checks that required external tools are available for each enabled
// backup target. It returns a slice of human-readable warning strings — one per
// missing dependency. Warnings are non-fatal; the caller should display them but
// still attempt the sync so the actual operation error surfaces if needed.
func Preflight(cfg *config.Config) []string {
	var warnings []string

	check := func(binary, context string) {
		if _, err := exec.LookPath(binary); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: '%s' not found in PATH — install %s first", context, binary, binary))
		}
	}

	if cfg.Git.Enabled {
		check("git", "git backup")
		check("rsync", "git backup")
		if cfg.Git.Repo == "" {
			warnings = append(warnings, "git backup: no repository URL configured — run 'sv init' to set one")
		}
	}

	if cfg.S3.Enabled {
		check("aws", "s3 backup")
		check("tar", "s3 backup")
	}

	if cfg.GCS.Enabled {
		check("gcloud", "gcs backup")
		check("tar", "gcs backup")
	}

	if cfg.Azure.Enabled {
		check("az", "azure backup")
		check("tar", "azure backup")
	}

	if cfg.ICloud.Enabled {
		home, _ := os.UserHomeDir()
		icloudBase := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
		if _, err := os.Stat(icloudBase); os.IsNotExist(err) {
			warnings = append(warnings, "icloud backup: iCloud Drive directory not found — is iCloud Drive enabled?")
		}
		check("tar", "icloud backup")
	}

	if cfg.TimeMachine.Enabled {
		check("tmutil", "time machine backup")
	}

	return warnings
}

func Run(cfg *config.Config) error {
	var errs []error

	if cfg.Git.Enabled {
		if err := syncGit(cfg); err != nil {
			fmt.Printf("  ✗ git sync failed: %s\n", err)
			errs = append(errs, fmt.Errorf("git sync: %w", err))
		}
	}
	if cfg.S3.Enabled {
		if err := syncS3(cfg); err != nil {
			fmt.Printf("  ✗ s3 sync failed: %s\n", err)
			errs = append(errs, fmt.Errorf("s3 sync: %w", err))
		}
	}
	if cfg.GCS.Enabled {
		if err := syncGCS(cfg); err != nil {
			fmt.Printf("  ✗ gcs sync failed: %s\n", err)
			errs = append(errs, fmt.Errorf("gcs sync: %w", err))
		}
	}
	if cfg.Azure.Enabled {
		if err := syncAzure(cfg); err != nil {
			fmt.Printf("  ✗ azure sync failed: %s\n", err)
			errs = append(errs, fmt.Errorf("azure sync: %w", err))
		}
	}
	if cfg.ICloud.Enabled {
		if err := syncICloud(cfg); err != nil {
			fmt.Printf("  ✗ icloud sync failed: %s\n", err)
			errs = append(errs, fmt.Errorf("icloud sync: %w", err))
		}
	}
	if cfg.TimeMachine.Enabled {
		if err := verifyTimeMachine(cfg); err != nil {
			fmt.Printf("  ⚠ Time Machine: %s\n", err)
		}
	}

	return errors.Join(errs...)
}

func syncGit(cfg *config.Config) error {
	repoDir := detect.ExpandHome(cfg.Git.LocalPath)

	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		fmt.Printf("  Cloning %s...\n", cfg.Git.Repo)
		if err := run("git", "clone", cfg.Git.Repo, repoDir); err != nil {
			return fmt.Errorf("clone: %w", err)
		}
	}

	// Check whether the repo has any commits. A freshly cloned empty repo has no
	// HEAD, so git pull --rebase would fail. In that case we skip stash/pull and
	// just do the initial commit+push below.
	hasCommits := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD").Run() == nil

	branch := detectDefaultBranch(repoDir)

	if hasCommits {
		stashOut, _ := runInCapture(repoDir, "git", "stash", "push", "--include-untracked", "-m",
			fmt.Sprintf("sv auto-stash %s", time.Now().Format("2006-01-02 15:04")))
		stashed := !strings.Contains(stashOut, "No local changes to save")

		if err := runIn(repoDir, "git", "pull", "--rebase", "origin", branch); err != nil {
			if stashed {
				_ = runIn(repoDir, "git", "stash", "pop")
			}
			return fmt.Errorf("pull: %w", err)
		}

		if stashed {
			_ = runIn(repoDir, "git", "stash", "pop")
		}
	}

	for name, tool := range cfg.Tools {
		if !tool.Enabled {
			continue
		}
		if err := syncTool(repoDir, name, tool); err != nil {
			return fmt.Errorf("sync %s: %w", name, err)
		}
	}

	if err := runIn(repoDir, "git", "add", "-A"); err != nil {
		return err
	}

	if exec.Command("git", "-C", repoDir, "diff", "--cached", "--quiet").Run() == nil {
		fmt.Println("  No changes to sync")
		return nil
	}

	msg := fmt.Sprintf("sv sync %s", time.Now().Format("2006-01-02 15:04"))
	if err := runIn(repoDir, "git", "commit", "-m", msg); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	if err := runIn(repoDir, "git", "push", "origin", branch); err != nil {
		return fmt.Errorf("push: %w", err)
	}

	fmt.Printf("  Pushed to %s (%s)\n", cfg.Git.Provider, cfg.Git.Repo)
	return nil
}

func detectDefaultBranch(repoDir string) string {
	out, err := exec.Command("git", "-C", repoDir, "symbolic-ref", "refs/remotes/origin/HEAD").Output()
	if err == nil {
		ref := strings.TrimSpace(string(out))
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return "main"
}

func syncTool(repoDir, name string, tool config.ToolConfig) error {
	var toolDef *detect.Tool
	for _, t := range detect.KnownTools {
		if t.Name == name {
			toolDef = &t
			break
		}
	}
	if toolDef == nil {
		return nil
	}

	enabledCategories := make(map[string]bool)
	for _, c := range tool.Categories {
		enabledCategories[c] = true
	}

	for _, bp := range toolDef.Paths {
		if !enabledCategories[string(bp.Category)] {
			continue
		}

		src := detect.ExpandHome(bp.Path)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}

		destDir := filepath.Join(repoDir, name, string(bp.Category))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return err
		}

		if bp.Pattern != "" {
			if err := copyGlob(src, bp.Pattern, destDir); err != nil {
				return err
			}
		} else {
			info, _ := os.Stat(src)
			if info != nil && info.IsDir() {
				dest := filepath.Join(repoDir, name, string(bp.Category), filepath.Base(src))
				if err := run("rsync", "-a", "--delete", src+"/", dest+"/"); err != nil {
					return err
				}
			} else {
				dest := filepath.Join(destDir, filepath.Base(src))
				if err := copyFile(src, dest); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func syncCloudConversations(cfg *config.Config, uploadFn func(archive, key string) error) error {
	for name, tool := range cfg.Tools {
		if !tool.Enabled {
			continue
		}

		hasConversations := false
		for _, c := range tool.Categories {
			if c == "conversations" {
				hasConversations = true
				break
			}
		}
		if !hasConversations {
			continue
		}

		var toolDef *detect.Tool
		for _, t := range detect.KnownTools {
			if t.Name == name {
				toolDef = &t
				break
			}
		}
		if toolDef == nil {
			continue
		}

		for _, bp := range toolDef.Paths {
			if bp.Category != detect.CategoryConversations {
				continue
			}

			src := detect.ExpandHome(bp.Path)
			if _, err := os.Stat(src); os.IsNotExist(err) {
				continue
			}

			if err := compressAndUpload(name, toolDef.Description, src, uploadFn); err != nil {
				return err
			}
		}
	}
	return nil
}

func compressAndUpload(name, description, src string, uploadFn func(archive, key string) error) error {
	datestamp := time.Now().Format("20060102")
	archive := filepath.Join(os.TempDir(), fmt.Sprintf("%s-conversations-%s.tar.gz", name, datestamp))
	defer os.Remove(archive)

	fmt.Printf("  Compressing %s conversations...\n", description)
	if err := run("tar", "czf", archive, "-C", filepath.Dir(src), filepath.Base(src)); err != nil {
		return fmt.Errorf("tar %s: %w", name, err)
	}

	key := fmt.Sprintf("%s-conversations-%s.tar.gz", name, datestamp)
	if err := uploadFn(archive, key); err != nil {
		return err
	}

	fmt.Printf("  Uploaded %s\n", key)
	return nil
}

func syncS3(cfg *config.Config) error {
	return syncCloudConversations(cfg, func(archive, key string) error {
		fmt.Printf("  Uploading to s3://%s/%s...\n", cfg.S3.Bucket, key)
		args := []string{"s3", "cp", archive,
			fmt.Sprintf("s3://%s/%s", cfg.S3.Bucket, key), "--quiet"}
		if cfg.S3.Profile != "" {
			args = append(args, "--profile", cfg.S3.Profile)
		}
		if cfg.S3.Region != "" {
			args = append(args, "--region", cfg.S3.Region)
		}
		return run("aws", args...)
	})
}

func syncGCS(cfg *config.Config) error {
	return syncCloudConversations(cfg, func(archive, key string) error {
		dest := fmt.Sprintf("gs://%s/%s", cfg.GCS.Bucket, key)
		fmt.Printf("  Uploading to %s...\n", dest)
		args := []string{"--quiet", "storage", "cp", archive, dest}
		if cfg.GCS.Project != "" {
			args = append(args, "--project", cfg.GCS.Project)
		}
		return run("gcloud", args...)
	})
}

func syncAzure(cfg *config.Config) error {
	return syncCloudConversations(cfg, func(archive, key string) error {
		fmt.Printf("  Uploading to azure://%s/%s...\n", cfg.Azure.Container, key)
		return run("az", "storage", "blob", "upload",
			"--container-name", cfg.Azure.Container,
			"--account-name", cfg.Azure.StorageAcct,
			"--name", key,
			"--file", archive,
			"--overwrite",
			"--only-show-errors")
	})
}

func syncICloud(cfg *config.Config) error {
	home, _ := os.UserHomeDir()
	icloudDir := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs", "sv")
	if err := os.MkdirAll(icloudDir, 0755); err != nil {
		return fmt.Errorf("create icloud dir: %w", err)
	}

	return syncCloudConversations(cfg, func(archive, key string) error {
		dest := filepath.Join(icloudDir, key)
		fmt.Printf("  Copying to iCloud Drive: %s...\n", key)
		return copyFile(archive, dest)
	})
}

func verifyTimeMachine(cfg *config.Config) error {
	configDir := config.Dir()
	out, err := exec.Command("tmutil", "isexcluded", configDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not check time machine status: %w", err)
	}
	if strings.Contains(string(out), "[Excluded]") {
		return fmt.Errorf("%s is excluded from time machine — run: tmutil removeexclusion -p %s", configDir, configDir)
	}
	fmt.Printf("  Time Machine: %s is included in backups\n", configDir)

	for name, tool := range cfg.Tools {
		if !tool.Enabled {
			continue
		}
		for _, t := range detect.KnownTools {
			if t.Name == name {
				dir := detect.ExpandHome(t.Dir)
				exOut, _ := exec.Command("tmutil", "isexcluded", dir).CombinedOutput()
				if strings.Contains(string(exOut), "[Excluded]") {
					fmt.Printf("  ⚠ %s (%s) is excluded from Time Machine\n", t.Description, dir)
				} else {
					fmt.Printf("  Time Machine: %s (%s) ✓\n", t.Description, dir)
				}
				break
			}
		}
	}
	return nil
}

// copyGlob walks root and copies files whose relative path matches pattern.
// Pattern uses filepath.Match syntax and may contain path separators (e.g.
// "*/memory/*.md"). For multi-segment patterns, matching is attempted at every
// suffix of the relative path so that deeply nested files can still match.
func copyGlob(root, pattern, destDir string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)

		if matchGlob(pattern, rel) {
			dest := filepath.Join(destDir, rel)
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			return copyFile(path, dest)
		}
		return nil
	})
}

// matchGlob checks whether rel matches pattern. It tries the full relative
// path first, then progressively shorter suffixes (dropping leading segments)
// so that a pattern like "*/memory/*.md" can match "foo/bar/memory/note.md".
func matchGlob(pattern, rel string) bool {
	// Normalize to forward slashes for consistent matching.
	rel = filepath.ToSlash(rel)
	pattern = filepath.ToSlash(pattern)

	if m, _ := filepath.Match(pattern, rel); m {
		return true
	}

	// Try suffix matches: for rel "a/b/c/d.md", try "b/c/d.md", "c/d.md", "d.md".
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		suffix := strings.Join(parts[i:], "/")
		if m, _ := filepath.Match(pattern, suffix); m {
			return true
		}
	}
	return false
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func run(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runIn(dir, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runInCapture(dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
