package tui

import "strings"

// fitHeight clamps a rendered block to an exact number of rows, truncating and
// padding as needed, and truncates each line to width to prevent wrapping.
func fitHeight(body string, height, width int) string {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		if i < len(lines) {
			out = append(out, truncate(lines[i], width))
		} else {
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}

// overlayCentered composites fg as a centered panel over bg. Rows covered by the
// panel are replaced entirely, which keeps ANSI styling intact (no rune slicing
// of styled lines).
func overlayCentered(bg, fg string, width, height int) string {
	bgLines := normalizeLines(bg, height)
	fgLines := strings.Split(fg, "\n")
	fgWidth := maxLineWidth(fgLines)
	if fgWidth > width-2 {
		fgWidth = width - 2
	}
	left := (width - fgWidth) / 2
	if left < 0 {
		left = 0
	}
	top := (height - len(fgLines)) / 2
	if top < 0 {
		top = 0
	}
	pad := strings.Repeat(" ", left)
	for i, line := range fgLines {
		row := top + i
		if row >= len(bgLines) {
			break
		}
		bgLines[row] = pad + line
	}
	return strings.Join(bgLines, "\n")
}

// overlayBottomRight places fg at the bottom-right, just above the bottom bar.
func overlayBottomRight(bg, fg string, width, height int) string {
	bgLines := normalizeLines(bg, height)
	fgLines := strings.Split(fg, "\n")
	fgWidth := maxLineWidth(fgLines)
	left := width - fgWidth - 1
	if left < 0 {
		left = 0
	}
	// End two rows above the very bottom (preserving the bottom bar).
	top := height - 2 - len(fgLines)
	if top < 0 {
		top = 0
	}
	pad := strings.Repeat(" ", left)
	for i, line := range fgLines {
		row := top + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		bgLines[row] = pad + line
	}
	return strings.Join(bgLines, "\n")
}

func normalizeLines(body string, height int) []string {
	lines := strings.Split(body, "\n")
	if len(lines) < height {
		for len(lines) < height {
			lines = append(lines, "")
		}
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

func maxLineWidth(lines []string) int {
	max := 0
	for _, line := range lines {
		if w := displayWidth(line); w > max {
			max = w
		}
	}
	return max
}
