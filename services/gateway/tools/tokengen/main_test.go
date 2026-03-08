package main

import (
	"testing"
	"time"
)

func TestParseDayDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"0d", 0, false},
		{"d", 0, true},   // too short after stripping 'd'... actually len("d")=1 < 2
		{"x", 0, true},   // too short
		{"", 0, true},    // too short
		{"7x", 0, true},  // not 'd' suffix
		{"abc", 0, true}, // not 'd' suffix
		{"12", 0, true},  // no 'd' suffix
		{"a7d", 0, true}, // non-digit before 'd'
	}

	for _, tt := range tests {
		got, err := parseDayDuration(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseDayDuration(%q) = %v, want error", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDayDuration(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseDayDuration(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
