package schedule

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

const plistLabel = "com.skill-vault.sync"

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>

    <key>ProgramArguments</key>
    <array>
        <string>{{.Binary}}</string>
        <string>sync</string>
    </array>

    <key>StartInterval</key>
    <integer>{{.IntervalSeconds}}</integer>

    <key>StandardOutPath</key>
    <string>{{.LogDir}}/sync.log</string>
    <key>StandardErrorPath</key>
    <string>{{.LogDir}}/sync-error.log</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin</string>
        <key>HOME</key>
        <string>{{.Home}}</string>
    </dict>
</dict>
</plist>
`

type plistData struct {
	Label           string
	Binary          string
	IntervalSeconds int
	LogDir          string
	Home            string
}

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", plistLabel+".plist")
}

func Install(intervalStr string) error {
	seconds, err := parseInterval(intervalStr)
	if err != nil {
		return err
	}

	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".skill-vault")
	os.MkdirAll(logDir, 0755)

	data := plistData{
		Label:           plistLabel,
		Binary:          binary,
		IntervalSeconds: seconds,
		LogDir:          logDir,
		Home:            home,
	}

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return err
	}

	path := plistPath()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return err
	}

	_ = exec.Command("launchctl", "unload", path).Run()
	if err := exec.Command("launchctl", "load", path).Run(); err != nil {
		return fmt.Errorf("launchctl load: %w", err)
	}

	return nil
}

func Uninstall() error {
	path := plistPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("no launchd job installed")
	}
	if err := exec.Command("launchctl", "unload", path).Run(); err != nil {
		return fmt.Errorf("launchctl unload: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove plist: %w", err)
	}
	return nil
}

func IsInstalled() bool {
	_, err := os.Stat(plistPath())
	return err == nil
}

func Status() string {
	if !IsInstalled() {
		return "not installed"
	}
	out, err := exec.Command("launchctl", "list", plistLabel).CombinedOutput()
	if err != nil {
		return "installed but not loaded"
	}
	output := string(out)
	if strings.Contains(output, "\"PID\"") {
		return "active"
	}
	return "loaded (idle)"
}

func LastRun() string {
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, ".skill-vault", "sync.log")
	info, err := os.Stat(logPath)
	if err != nil {
		return "never"
	}
	return info.ModTime().Format(time.RFC3339)
}

func parseInterval(s string) (int, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid interval %q (use e.g. 24h, 12h, 6h, 1h): %w", s, err)
	}
	secs := int(d.Seconds())
	if secs < 3600 {
		return 0, fmt.Errorf("minimum interval is 1h")
	}
	return secs, nil
}
