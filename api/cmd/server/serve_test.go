package main

import (
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},

		// Case-insensitive.
		{"DEBUG", slog.LevelDebug},
		{"Warn", slog.LevelWarn},
		{"ERROR", slog.LevelError},

		// Surrounding whitespace is trimmed.
		{" debug ", slog.LevelDebug},
		{"\twarn\n", slog.LevelWarn},

		// Empty and unrecognized values fall back to Info.
		{"", slog.LevelInfo},
		{"verbose", slog.LevelInfo},
		{"debg", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := parseLevel(tt.in); got != tt.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty string is nil", "", nil},
		{"single value", "a", []string{"a"}},
		{"two values", "a,b", []string{"a", "b"}},
		{"surrounding whitespace trimmed", " a , b ", []string{"a", "b"}},
		{"tabs and newlines trimmed", "\ta\n,\tb\n", []string{"a", "b"}},
		{"empty entries dropped", "a,,b", []string{"a", "b"}},
		{"trailing comma dropped", "a,b,", []string{"a", "b"}},
		{"leading comma dropped", ",a,b", []string{"a", "b"}},
		{"whitespace-only entries dropped", "a, ,b", []string{"a", "b"}},
		{"only separators yields nil", " , , ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCSV(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("splitCSV(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitCSV(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}
