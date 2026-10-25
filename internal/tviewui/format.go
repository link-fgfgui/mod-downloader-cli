package tviewui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// formatSpeed renders "x.xx MiB/s".
func formatSpeed(bytesPerSecond int64) string {
	if bytesPerSecond <= 0 {
		return "0.00 MiB/s"
	}
	return fmt.Sprintf("%.2f MiB/s", float64(bytesPerSecond)/(1024*1024))
}

// formatPercent renders a whole-number percentage (empty when total is unknown).
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

// compactNumber mirrors the frontend K/M/B formatting for usage stats.
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

// itoa formats an int without pulling in strconv in hot render paths.
func itoa(v int) string { return strconv.Itoa(v) }

// maxInt returns the larger value.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// minInt returns the smaller value.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// clampRange clamps v into [lo, hi].
func clampRange(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// checkBox renders the three-cell checkbox marker used by selectable rows.
// The box glyphs used before (☐/▣) are "ambiguous width" characters that CJK
// terminals draw at different widths, which shifted the row text when toggled.
func checkBox(selected bool) string {
	if selected {
		return "[X]"
	}
	return "[ ]"
}

// checkBoxTagged is checkBox for lists that parse style tags.
func checkBoxTagged(selected bool) string { return tview.Escape(checkBox(selected)) }

// truncate shortens a plain (untagged) string to width cells.
func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if width <= 1 {
		if tviewWidth(value) <= width {
			return value
		}
		return ""
	}
	// Measure each rune once. Re-measuring the whole prefix after every rune
	// made long descriptions cost O(n^2) uniseg work, which dominated refreshes
	// of the result list.
	used := 0
	cutAt, over := -1, -1
	for i, r := range value {
		w := runewidth.RuneWidth(r)
		if cutAt < 0 && used+w > width-1 {
			cutAt = i
		}
		if over < 0 && used+w > width {
			over = i
		}
		used += w
	}
	if over < 0 {
		return value
	}
	if cutAt < 0 {
		cutAt = over
	}
	return value[:cutAt] + "…"
}

// tviewWidth measures the display width of text.
func tviewWidth(value string) int {
	return tview.TaggedStringWidth(value)
}
