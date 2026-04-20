package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigRoundTrip(t *testing.T) {
	original := &Config{
		Tools: map[string]ToolConfig{
			"claude": {Enabled: true, Categories: []string{"skills", "config"}},
			"cursor": {Enabled: false, Categories: []string{"rules"}},
		},
		Git: GitConfig{
			Enabled:   true,
			Provider:  "github",
			Repo:      "user/skill-vault-backup",
			LocalPath: "/tmp/backup",
		},
		S3: S3Config{
			Enabled: true,
			Bucket:  "my-bucket",
			Profile: "default",
			Region:  "us-east-1",
		},
		GCS: GCSConfig{
			Enabled: false,
			Bucket:  "gcs-bucket",
			Project: "my-project",
		},
		Azure: AzureConfig{
			Enabled:       false,
			Container:     "az-container",
			StorageAcct:   "mystorageaccount",
			ResourceGroup: "my-rg",
		},
		ICloud: ICloudConfig{Enabled: true},
		TimeMachine: TimeMachineConfig{Enabled: false},
		Schedule: ScheduleConfig{
			Enabled:  true,
			Interval: "24h",
		},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	var loaded Config
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if loaded.Git.Provider != original.Git.Provider {
		t.Errorf("Git.Provider: got %q, want %q", loaded.Git.Provider, original.Git.Provider)
	}
	if loaded.Git.Repo != original.Git.Repo {
		t.Errorf("Git.Repo: got %q, want %q", loaded.Git.Repo, original.Git.Repo)
	}
	if loaded.S3.Bucket != original.S3.Bucket {
		t.Errorf("S3.Bucket: got %q, want %q", loaded.S3.Bucket, original.S3.Bucket)
	}
	if loaded.S3.Region != original.S3.Region {
		t.Errorf("S3.Region: got %q, want %q", loaded.S3.Region, original.S3.Region)
	}
	if loaded.Schedule.Interval != original.Schedule.Interval {
		t.Errorf("Schedule.Interval: got %q, want %q", loaded.Schedule.Interval, original.Schedule.Interval)
	}
	if !loaded.ICloud.Enabled {
		t.Errorf("ICloud.Enabled: got false, want true")
	}
	if loaded.TimeMachine.Enabled {
		t.Errorf("TimeMachine.Enabled: got true, want false")
	}

	claudeTool, ok := loaded.Tools["claude"]
	if !ok {
		t.Fatal("Tools[\"claude\"] not found after round-trip")
	}
	if !claudeTool.Enabled {
		t.Errorf("Tools[\"claude\"].Enabled: got false, want true")
	}
	if len(claudeTool.Categories) != 2 {
		t.Errorf("Tools[\"claude\"].Categories: got %v, want 2 items", claudeTool.Categories)
	}

	cursorTool, ok := loaded.Tools["cursor"]
	if !ok {
		t.Fatal("Tools[\"cursor\"] not found after round-trip")
	}
	if cursorTool.Enabled {
		t.Errorf("Tools[\"cursor\"].Enabled: got true, want false")
	}
}

func TestDirReturnsSuffix(t *testing.T) {
	d := Dir()
	if !strings.HasSuffix(d, ".skill-vault") {
		t.Errorf("Dir() = %q, want suffix \".skill-vault\"", d)
	}
}

func TestPathReturnsSuffix(t *testing.T) {
	p := Path()
	if !strings.HasSuffix(p, ".skill-vault/config.yaml") {
		t.Errorf("Path() = %q, want suffix \".skill-vault/config.yaml\"", p)
	}
}

func TestExistsReturnsFalseForNonexistent(t *testing.T) {
	if Exists() {
		t.Skip("config file exists in home dir; skipping nonexistent test")
	}
}

func TestConfigEmptyToolsMap(t *testing.T) {
	original := &Config{
		Schedule: ScheduleConfig{Enabled: false, Interval: "6h"},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	var loaded Config
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if loaded.Schedule.Interval != "6h" {
		t.Errorf("Schedule.Interval: got %q, want \"6h\"", loaded.Schedule.Interval)
	}
}
