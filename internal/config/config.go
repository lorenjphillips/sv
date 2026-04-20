package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ToolConfig struct {
	Enabled    bool     `yaml:"enabled"`
	Categories []string `yaml:"categories"`
}

type GitConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Provider  string `yaml:"provider"`
	Repo      string `yaml:"repo"`
	LocalPath string `yaml:"local_path"`
}

type S3Config struct {
	Enabled bool   `yaml:"enabled"`
	Bucket  string `yaml:"bucket"`
	Profile string `yaml:"profile"`
	Region  string `yaml:"region"`
}

type GCSConfig struct {
	Enabled bool   `yaml:"enabled"`
	Bucket  string `yaml:"bucket"`
	Project string `yaml:"project"`
}

type AzureConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Container     string `yaml:"container"`
	StorageAcct   string `yaml:"storage_account"`
	ResourceGroup string `yaml:"resource_group"`
}

type ICloudConfig struct {
	Enabled bool `yaml:"enabled"`
}

type TimeMachineConfig struct {
	Enabled bool `yaml:"enabled"`
}

type ScheduleConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Interval string `yaml:"interval"`
}

type Config struct {
	Tools       map[string]ToolConfig `yaml:"tools"`
	Git         GitConfig             `yaml:"git"`
	S3          S3Config              `yaml:"s3"`
	GCS         GCSConfig             `yaml:"gcs,omitempty"`
	Azure       AzureConfig           `yaml:"azure,omitempty"`
	ICloud      ICloudConfig          `yaml:"icloud,omitempty"`
	TimeMachine TimeMachineConfig     `yaml:"time_machine,omitempty"`
	Schedule    ScheduleConfig        `yaml:"schedule"`
}

func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sv")
}

func Path() string {
	return filepath.Join(Dir(), "config.yaml")
}

func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), data, 0644)
}

func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}
