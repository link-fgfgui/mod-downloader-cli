package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// formatBytes renders a byte count using binary units.
func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	value := float64(bytes)
	idx := 0
	for value >= 1024 && idx < len(units)-1 {
		value /= 1024
		idx++
	}
	if idx == 0 {
		return fmt.Sprintf("%d %s", bytes, units[idx])
	}
	return fmt.Sprintf("%.1f %s", value, units[idx])
}

// formatSpeed mirrors formatSpeed(bytesPerSecond) -> "x.xx MiB/s".
func formatSpeed(bytesPerSecond int64) string {
	if bytesPerSecond <= 0 {
		return "0.00 MiB/s"
	}
	mib := float64(bytesPerSecond) / (1024 * 1024)
	return fmt.Sprintf("%.2f MiB/s", mib)
}

// formatPercent renders a whole-number percentage (0 when the total is unknown).
func formatPercent(complete, total int64) string {
	if total <= 0 {
		return ""
	}
	pct := float64(complete) / float64(total) * 100
	if pct > 100 {
		pct = 100
	}
	return strconv.Itoa(int(math.Round(pct))) + "%"
}

// compactNumber mirrors the frontend K/M compact formatting for usage stats.
func compactNumber(value int64) string {
	abs := float64(value)
	if abs < 1000 {
		return strconv.FormatInt(value, 10)
	}
	suffixes := []string{"K", "M", "B", "T"}
	idx := -1
	for abs >= 1000 && idx < len(suffixes)-1 {
		abs /= 1000
		idx++
	}
	// Keep one decimal for values below 10 (e.g. 1.2K) to match the GUI.
	if abs < 10 {
		return trimTrailingZero(fmt.Sprintf("%.1f", abs)) + suffixes[idx]
	}
	return trimTrailingZero(fmt.Sprintf("%.0f", abs)) + suffixes[idx]
}

func trimTrailingZero(value string) string {
	if strings.HasSuffix(value, ".0") {
		return strings.TrimSuffix(value, ".0")
	}
	return value
}

// progressBar renders a fixed-width textual progress bar.
func progressBar(complete, total int64, width int) string {
	if width < 4 {
		width = 4
	}
	if total <= 0 {
		// Unknown size: render an indeterminate dotted bar.
		filled := width / 2
		return strings.Repeat("·", filled) + strings.Repeat(" ", width-filled)
	}
	ratio := float64(complete) / float64(total)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(math.Round(ratio * float64(width)))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// truncate shortens a string to width runes, appending an ellipsis when cut.
func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

// padRight pads a string to width using display width compensation for CJK.
func padRight(value string, width int) string {
	w := displayWidth(value)
	if w >= width {
		return value
	}
	return value + strings.Repeat(" ", width-w)
}

// displayWidth approximates terminal display width (wide CJK runes count 2).
func displayWidth(value string) int {
	width := 0
	for _, r := range value {
		if r >= 0x1100 && (r <= 0x115F || (r >= 0x2E80 && r <= 0xA4CF) || (r >= 0xAC00 && r <= 0xD7A3) ||
			(r >= 0xF900 && r <= 0xFAFF) || (r >= 0xFE30 && r <= 0xFE6F) || (r >= 0xFF00 && r <= 0xFF60) ||
			(r >= 0xFFE0 && r <= 0xFFE6) || (r >= 0x20000 && r <= 0x3FFFD)) {
			width += 2
		} else {
			width++
		}
	}
	return width
}
