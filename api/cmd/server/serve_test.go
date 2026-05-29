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
