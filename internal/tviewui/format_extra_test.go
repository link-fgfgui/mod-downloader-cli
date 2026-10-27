package tviewui

import (
	"strings"
	"testing"
)

func TestFormatSpeed(t *testing.T) {
	tests := []struct {
		name  string
		value int64
		want  string
	}{
		{"zero", 0, "0.00 MiB/s"},
		{"negative", -10, "0.00 MiB/s"},
		{"one mib", 1024 * 1024, "1.00 MiB/s"},
		{"one and a half", 1024*1024 + 512*1024, "1.50 MiB/s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatSpeed(tt.value); got != tt.want {
				t.Fatalf("formatSpeed(%d)=%q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestFormatPercent(t *testing.T) {
	tests := []struct {
		name     string
		complete int64
		total    int64
		want     string
	}{
		{"unknown total", 10, 0, ""},
		{"negative total", 10, -5, ""},
		{"half", 50, 100, "50%"},
		{"round", 1, 3, "33%"},
		{"overcomplete", 150, 100, "100%"},
		{"zero complete", 0, 100, "0%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPercent(tt.complete, tt.total); got != tt.want {
				t.Fatalf("formatPercent(%d,%d)=%q, want %q", tt.complete, tt.total, got, tt.want)
			}
		})
	}
}

func TestCompactNumber(t *testing.T) {
	tests := []struct {
		value int64
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1K"},
		{1500, "1.5K"},
		{12_000, "12K"},
		{1_000_000, "1M"},
		{1_500_000, "1.5M"},
		{2_000_000_000, "2B"},
		{3_000_000_000_000, "3T"},
		{-5, "-5"},
	}
	for _, tt := range tests {
		if got := compactNumber(tt.value); got != tt.want {
			t.Fatalf("compactNumber(%d)=%q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestTrimTrailingZero(t *testing.T) {
	if got := trimTrailingZero("1.0"); got != "1" {
		t.Fatalf("trimTrailingZero(1.0)=%q", got)
	}
	if got := trimTrailingZero("1.5"); got != "1.5" {
		t.Fatalf("trimTrailingZero(1.5)=%q", got)
	}
	if got := trimTrailingZero("10"); got != "10" {
		t.Fatalf("trimTrailingZero(10)=%q", got)
	}
}

func TestProgressBar(t *testing.T) {
	if got, want := progressBar(0, 0, 8), "····    "; got != want {
		t.Fatalf("unknown total bar=%q, want %q", got, want)
	}
	if got, want := progressBar(50, 100, 10), "█████░░░░░"; got != want {
		t.Fatalf("half bar=%q, want %q", got, want)
	}
	if got, want := progressBar(200, 100, 4), "████"; got != want {
		t.Fatalf("overcomplete bar=%q, want %q", got, want)
	}
	if got, want := progressBar(-5, 100, 4), "░░░░"; got != want {
		t.Fatalf("negative bar=%q, want %q", got, want)
	}
	// Widths below 4 are clamped so the bar stays readable.
	if got := progressBar(0, 100, 1); tviewWidth(got) != 4 {
		t.Fatalf("narrow bar width=%d, want 4 (%q)", tviewWidth(got), got)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		value string
		width int
		want  string
	}{
		{"zero width", "abc", 0, ""},
		{"single cell fits", "a", 1, "a"},
		{"single cell too narrow", "ab", 1, ""},
		{"exact ascii", "abc", 3, "abc"},
		{"truncate ascii", "abcdef", 3, "ab…"},
		{"cjk exact", "你好", 4, "你好"},
		{"cjk truncate", "你好世界", 3, "你…"},
		{"empty", "", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.value, tt.width); got != tt.want {
				t.Fatalf("truncate(%q,%d)=%q, want %q", tt.value, tt.width, got, tt.want)
			}
		})
	}
	// A truncated string must never exceed the requested cell width.
	for width := 1; width <= 12; width++ {
		got := truncate("你好abc世界", width)
		if w := tviewWidth(got); w > width {
			t.Fatalf("truncate width=%d produced %d cells: %q", width, w, got)
		}
	}
}

func TestCheckBox(t *testing.T) {
	if checkBox(true) != "[X]" || checkBox(false) != "[ ]" {
		t.Fatalf("checkBox glyphs changed: %q %q", checkBox(true), checkBox(false))
	}
	// Tagged rows must escape the brackets so tview does not parse them as tags.
	if got := checkBoxTagged(true); !strings.Contains(got, "[") || got == "[X]" {
		t.Fatalf("checkBoxTagged(true)=%q, want escaped brackets", got)
	}
}

func TestIntHelpers(t *testing.T) {
	if maxInt(3, 5) != 5 || maxInt(-1, -2) != -1 {
		t.Fatal("maxInt wrong")
	}
	if minInt(3, 5) != 3 || minInt(-1, -2) != -2 {
		t.Fatal("minInt wrong")
	}
	if clampRange(10, 1, 5) != 5 || clampRange(-3, 1, 5) != 1 || clampRange(3, 1, 5) != 3 {
		t.Fatal("clampRange wrong")
	}
	if itoa(42) != "42" {
		t.Fatal("itoa wrong")
	}
}
