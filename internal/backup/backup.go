package backup

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lorenjphillips/skill-vault/internal/config"
	"github.com/lorenjphillips/skill-vault/internal/detect"
)

func Run(cfg *config.Config) error {
	if cfg.Git.Enabled {
		if err := syncGit(cfg); err != nil {
			return fmt.Errorf("git sync: %w", err)
		}
	}
	if cfg.S3.Enabled {
		if err := syncS3(cfg); err != nil {
			return fmt.Errorf("s3 sync: %w", err)
		}
	}
	if cfg.GCS.Enabled {
		if err := syncGCS(cfg); err != nil {
			return fmt.Errorf("gcs sync: %w", err)
		}
	}
	if cfg.Azure.Enabled {
		if err := syncAzure(cfg); err != nil {
			return fmt.Errorf("azure sync: %w", err)
		}
	}
	if cfg.ICloud.Enabled {
		if err := syncICloud(cfg); err != nil {
			return fmt.Errorf("icloud sync: %w", err)
		}
	}
	if cfg.TimeMachine.Enabled {
		if err := verifyTimeMachine(cfg); err != nil {
			fmt.Printf("  ⚠ Time Machine: %s\n", err)
		}
	}
	return nil
}

func syncGit(cfg *config.Config) error {
	repoDir := detect.ExpandHome(cfg.Git.LocalPath)

	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		fmt.Printf("  Cloning %s...\n", cfg.Git.Repo)
		if err := run("git", "clone", cfg.Git.Repo, repoDir); err != nil {
			return fmt.Errorf("clone: %w", err)
		}
	}

	stashOut, _ := runInCapture(repoDir, "git", "stash", "push", "--include-untracked", "-m",
		fmt.Sprintf("skill-vault auto-stash %s", time.Now().Format("2006-01-02 15:04")))
	stashed := !strings.Contains(stashOut, "No local changes to save")

	branch := detectDefaultBranch(repoDir)
	if err := runIn(repoDir, "git", "pull", "--rebase", "origin", branch); err != nil {
		if stashed {
			_ = runIn(repoDir, "git", "stash", "pop")
		}
		return fmt.Errorf("pull: %w", err)
	}

	if stashed {
		_ = runIn(repoDir, "git", "stash", "drop")
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

	msg := fmt.Sprintf("skill-vault sync %s", time.Now().Format("2006-01-02 15:04"))
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

			datestamp := time.Now().Format("20060102")
			archive := filepath.Join(os.TempDir(), fmt.Sprintf("%s-conversations-%s.tar.gz",
				name, datestamp))
			defer os.Remove(archive)

			fmt.Printf("  Compressing %s conversations...\n", toolDef.Description)
			if err := run("tar", "czf", archive, "-C", filepath.Dir(src), filepath.Base(src)); err != nil {
				return fmt.Errorf("tar %s: %w", name, err)
			}

			key := fmt.Sprintf("%s-conversations-%s.tar.gz", name, datestamp)
			if err := uploadFn(archive, key); err != nil {
				return err
			}

			fmt.Printf("  Uploaded %s\n", key)
		}
	}
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
		args := []string{"storage", "cp", archive, dest, "--quiet"}
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
	icloudDir := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs", "skill-vault")
	os.MkdirAll(icloudDir, 0755)

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

func copyGlob(root, pattern, destDir string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		matched, _ := filepath.Match(pattern, rel)
		if !matched {
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) > 1 {
				matched, _ = filepath.Match(pattern, filepath.Join(parts[len(parts)-2], parts[len(parts)-1]))
			}
		}
		if !matched {
			for _, part := range strings.Split(pattern, "/") {
				if m, _ := filepath.Match(part, filepath.Base(path)); m {
					matched = true
					break
				}
			}
		}
		if matched {
			dest := filepath.Join(destDir, rel)
			os.MkdirAll(filepath.Dir(dest), 0755)
			return copyFile(path, dest)
		}
		return nil
	})
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runIn(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runInCapture(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
