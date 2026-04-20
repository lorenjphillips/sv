package schedule

import "testing"

func TestParseInterval(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		wantSec int
	}{
		{"1h", false, 3600},
		{"6h", false, 21600},
		{"24h", false, 86400},
		{"168h", false, 604800},
		{"2h30m", false, 9000},
		{"30m", true, 0},
		{"59m59s", true, 0},
		{"0s", true, 0},
		{"abc", true, 0},
		{"", true, 0},
		{"-1h", true, 0},
	}

	for _, tt := range tests {
		got, err := parseInterval(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseInterval(%q): expected error, got nil (result=%d)", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseInterval(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.wantSec {
			t.Errorf("parseInterval(%q) = %d, want %d", tt.input, got, tt.wantSec)
		}
	}
}
