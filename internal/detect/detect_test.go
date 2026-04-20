package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input string
		want  string
	}{
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"~/.config", filepath.Join(home, ".config")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
		{"~", "~"},
	}

	for _, tt := range tests {
		got := ExpandHome(tt.input)
		if got != tt.want {
			t.Errorf("ExpandHome(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatSize(t *testing.T) {
	const (
		kb = int64(1024)
		mb = kb * 1024
		gb = mb * 1024
	)

	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{kb, "1.0 KB"},
		{kb + 512, "1.5 KB"},
		{mb - 1, "1024.0 KB"},
		{mb, "1.0 MB"},
		{mb*2 + mb/2, "2.5 MB"},
		{gb - 1, "1024.0 MB"},
		{gb, "1.0 GB"},
		{gb * 3, "3.0 GB"},
	}

	for _, tt := range tests {
		got := FormatSize(tt.bytes)
		if got != tt.want {
			t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()

	files := map[string]int{
		"a.txt": 100,
		"b.txt": 200,
		"c.txt": 300,
	}
	var want int64
	for name, size := range files {
		data := make([]byte, size)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		want += int64(size)
	}

	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	subData := make([]byte, 50)
	if err := os.WriteFile(filepath.Join(subdir, "d.txt"), subData, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	want += 50

	got := dirSize(dir)
	if got != want {
		t.Errorf("dirSize(%q) = %d, want %d", dir, got, want)
	}
}

func TestExpandHomeTildeOnly(t *testing.T) {
	got := ExpandHome("~")
	if strings.HasPrefix(got, "/") {
		t.Errorf("ExpandHome(\"~\") should not expand bare tilde, got %q", got)
	}
}
