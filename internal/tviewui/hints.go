package tviewui

import "strings"

// hint is one segment of the bottom status bar. Segments with a Run action are
// clickable (and highlighted on hover-free click), so every shortcut shown in
// the bar can also be triggered with the mouse.
type hint struct {
	Text string
	Run  func()
}

// hintText is an informational segment with no mouse action.
func hintText(text string) hint { return hint{Text: text} }

// hintAction is a clickable segment.
func hintAction(text string, run func()) hint { return hint{Text: text, Run: run} }

// hintTextLine joins hints into a plain line for widgets that cannot render
// clickable regions.
func hintTextLine(hints []hint) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		parts = append(parts, h.Text)
	}
	return strings.Join(parts, " · ")
}
