// Package formatutil holds display formatting shared by the TUI and the
// one-shot CLI so both render the same progress and truncation output.
package formatutil

import (
	"fmt"

	"github.com/mattn/go-runewidth"
)

// HumanSpeed renders bytes/second as MiB/s.
func HumanSpeed(bytesPerSecond int64) string {
	if bytesPerSecond <= 0 {
		return "0.00 MiB/s"
	}
	return fmt.Sprintf("%.2f MiB/s", float64(bytesPerSecond)/(1024*1024))
}

// Truncate shortens plain text to width display cells, appending an ellipsis
// when the value does not fit.
func Truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if width <= 1 {
		if runewidth.StringWidth(value) <= width {
			return value
		}
		return ""
	}
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
